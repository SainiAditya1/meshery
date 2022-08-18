package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"cuelang.org/go/cue"
	"cuelang.org/go/cue/cuecontext"
	"github.com/layer5io/meshkit/utils"
)

type validationInputType string

const (
	jsontype       validationInputType = "JSON"
	yamltype       validationInputType = "YAML"
	jsonschematype validationInputType = "JSONSCHEMA"
	cuetype        validationInputType = "CUE"
)

type validationItem struct {
	Schema    string              `json:"schema"`
	Value     string              `json:"value"`
	ValueType validationInputType `json:"valueType,omitempty"`
}

type payload struct {
	ValidationItems map[string]validationItem `json:"validationItems"`
}

type validationResponse struct {
	IsValid bool   `json:"isValid"`
	Error   string `json:"error"`
}

type jsonSchemaValidationType struct {
	Schema string `json:"$schema,omitempty"`
}

func mmValidate(validationItems map[string]validationItem) map[string]validationResponse {
	validationResults := make(map[string]validationResponse, 0)
	for id, vi := range validationItems {
		// Parse the schema as CUE value
		schemaType := findSchemaType(vi.Schema)
		cueSchema, err := parseSchema(vi.Schema, schemaType)
		if err != nil {
			// if there is an error, push it into the map and continue
			validationResults[id] = validationResponse{IsValid: false, Error: err.Error()}
			continue
		}
		// Parse the value as CUE value
		cueValue, err := parseValue(vi.Value, vi.ValueType)
		if err != nil {
			// if there is an error, push it into the map and continue
			validationResults[id] = validationResponse{IsValid: false, Error: err.Error()}
			continue
		}
		// Validate the value against the schema
		isValid, err := utils.Validate(cueSchema, cueValue)
		if err != nil {
			validationResults[id] = validationResponse{IsValid: false, Error: err.Error()}
		}
		if isValid {
			// empty string means that the item is valid
			validationResults[id] = validationResponse{IsValid: true, Error: ""}
		}
	}
	return validationResults
}

func (h *Handler) MeshModelValidate(rw http.ResponseWriter, r *http.Request) {
	// Parse the request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		h.log.Error(ErrRequestBody(err))
		http.Error(rw, ErrRequestBody(err).Error(), http.StatusInternalServerError)
		return
	}
	// Unmarshal request body
	pld := payload{}
	err = json.Unmarshal(body, &pld)
	if err != nil {
		h.log.Error(ErrRequestBody(err))
		http.Error(rw, ErrRequestBody(err).Error(), http.StatusBadRequest)
		return
	}
	// Validate
	validationResults := mmValidate(pld.ValidationItems)
	// Send response
	rw.Header().Set("Content-Type", "application/json")
	err = json.NewEncoder(rw).Encode(struct {
		ValidationErrors map[string]validationResponse `json:"errors"`
	}{
		ValidationErrors: validationResults,
	})
	if err != nil {
		h.log.Error(ErrValidate(err))
		http.Error(rw, ErrValidate(err).Error(), http.StatusInternalServerError)
		return
	}
}

// if schema is not a JSONSCHEMA, we assume that it is CUE
func findSchemaType(schema string) validationInputType {
	jsValType := jsonSchemaValidationType{}
	err := json.Unmarshal([]byte(schema), &jsValType)
	if err != nil {
		// schema is not a valid JSON
		return cuetype
	}
	if jsValType.Schema == "" {
		return cuetype
	}
	return jsonschematype
}

// NOTE: does not return meshkit error - make sure to wrap it in meshkit errors before using
func parseSchema(schema string, schemaType validationInputType) (cue.Value, error) {
	if schemaType == jsonschematype {
		cueVal, err := utils.JsonSchemaToCue(schema)
		if err != nil {
			return cue.Value{}, err
		}
		return cueVal, nil
	}
	if schemaType == cuetype {
		// if not jsonschema, then it should be CUE
		cuectx := cuecontext.New()
		cueVal := cuectx.CompileString(schema)
		if cueVal.Err() != nil {
			return cue.Value{}, cueVal.Err()
		}
		return cueVal, nil
	}
	return cue.Value{}, fmt.Errorf("given schema is not in JSONSCHEMA or CUE format")
}

// NOTE: does not return meshkit error - make sure to wrap it in meshkit errors before using
func parseValue(value string, valueType validationInputType) (cue.Value, error) {
	if valueType == jsontype {
		cueVal, err := utils.JsonToCue([]byte(value))
		if err != nil {
			return cue.Value{}, err
		}
		return cueVal, nil
	}
	if valueType == yamltype {
		cueVal, err := utils.YamlToCue(value)
		if err != nil {
			return cue.Value{}, err
		}
		return cueVal, nil
	}
	if valueType == cuetype {
		cuectx := cuecontext.New()
		cueVal := cuectx.CompileString(value)
		if cueVal.Err() != nil {
			return cue.Value{}, cueVal.Err()
		}
		return cueVal, nil
	}
	return cue.Value{}, fmt.Errorf("given value is not in JSON,YAML or CUE format")
}
