// Copyright 2019 Kaleido

// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at

//     http://www.apache.org/licenses/LICENSE-2.0

// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package kldopenapi

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/accounts/abi"
	"github.com/go-openapi/jsonreference"
	"github.com/go-openapi/spec"
	"github.com/tidwall/gjson"
)

// ABI2Swagger is the main entry point for conversion
type ABI2Swagger struct {
	externalHost     string
	externalSchemes  []string
	externalRootPath string
}

// NewABI2Swagger constructor
func NewABI2Swagger(externalHost, externalRootPath string, externalSchemes []string) *ABI2Swagger {
	c := &ABI2Swagger{
		externalHost:     externalHost,
		externalRootPath: externalRootPath,
		externalSchemes:  externalSchemes,
	}
	if len(c.externalSchemes) == 0 {
		c.externalSchemes = []string{"http", "https"}
	}
	return c
}

// Gen4Instance generates OpenAPI for a single contract instance with an address
func (c *ABI2Swagger) Gen4Instance(basePath, name string, abi *abi.ABI, devdocsJSON string) *spec.Swagger {
	return c.convert(basePath, name, abi, devdocsJSON, true)
}

// Gen4Factory generates OpenAPI for a contract factory, with a constructor, and child methods on any addres
func (c *ABI2Swagger) Gen4Factory(basePath, name string, abi *abi.ABI, devdocsJSON string) *spec.Swagger {
	return c.convert(basePath, name, abi, devdocsJSON, false)
}

// convert does the conversion and fills in the details on the Swagger Schema
func (c *ABI2Swagger) convert(basePath, name string, abi *abi.ABI, devdocsJSON string, inst bool) *spec.Swagger {

	basePath = c.externalRootPath + basePath

	devdocs := gjson.Parse(devdocsJSON)

	paths := &spec.Paths{}
	paths.Paths = make(map[string]spec.PathItem)
	definitions := make(map[string]spec.Schema)
	parameters := c.getCommonParameters()
	c.buildDefinitionsAndPaths(inst, abi, definitions, paths.Paths, devdocs)
	return &spec.Swagger{
		SwaggerProps: spec.SwaggerProps{
			Swagger: "2.0",
			Info: &spec.Info{
				InfoProps: spec.InfoProps{
					Version:     "1.0",
					Title:       name,
					Description: devdocs.Get("details").String(),
				},
			},
			Host:        c.externalHost,
			Schemes:     c.externalSchemes,
			BasePath:    basePath,
			Paths:       paths,
			Definitions: definitions,
			Parameters:  parameters,
		},
	}
}

func (c *ABI2Swagger) buildDefinitionsAndPaths(inst bool, abi *abi.ABI, defs map[string]spec.Schema, paths map[string]spec.PathItem, devdocs gjson.Result) {
	methodsDocs := devdocs.Get("methods")
	if !inst {
		c.buildMethodDefinitionsAndPath(inst, defs, paths, "constructor", abi.Constructor, methodsDocs)
	}
	for _, method := range abi.Methods {
		c.buildMethodDefinitionsAndPath(inst, defs, paths, method.Name, method, methodsDocs)
	}
	errSchema := spec.Schema{
		SchemaProps: spec.SchemaProps{
			Properties: make(map[string]spec.Schema),
		},
	}
	errSchema.Properties["error"] = spec.Schema{
		SchemaProps: spec.SchemaProps{
			Description: "Error message",
			Type:        []string{"string"},
		},
	}
	defs["error"] = errSchema
}

func (c *ABI2Swagger) buildMethodDefinitionsAndPath(inst bool, defs map[string]spec.Schema, paths map[string]spec.PathItem, name string, method abi.Method, devdocs gjson.Result) {

	methodSig := name
	constructor := name == "constructor"
	path := "/"
	if !constructor {
		if inst {
			path = "/" + name
		} else {
			path = "/{address}/" + name
		}
		methodSig += "("
		for i, input := range method.Inputs {
			if i > 0 {
				methodSig += ","
			}
			methodSig += input.Type.String()
		}
		methodSig += ")"
	}
	search := strings.ReplaceAll(methodSig, "(", "\\(")
	search = strings.ReplaceAll(methodSig, ")", "\\)")
	methodDocs := devdocs.Get(search)

	inputSchema := url.QueryEscape(name) + "_inputs"
	outputSchema := url.QueryEscape(name) + "_outputs"
	c.buildArgumentsDefinition(defs, outputSchema, method.Outputs, true, methodDocs)
	pathItem := spec.PathItem{}
	if name != "constructor" {
		pathItem.Get = c.buildGETPath(outputSchema, inst, method, methodSig, methodDocs)
	}
	c.buildArgumentsDefinition(defs, inputSchema, method.Inputs, false, methodDocs)
	pathItem.Post = c.buildPOSTPath(inputSchema, outputSchema, inst, constructor, method, methodSig, methodDocs)
	paths[path] = pathItem

	return
}

func (c *ABI2Swagger) getCommonParameters() map[string]spec.Parameter {
	params := make(map[string]spec.Parameter)
	params["fromParam"] = spec.Parameter{
		ParamProps: spec.ParamProps{
			Description: "The 'from' address - 'x-kaleido-from' header can also be used",
			Name:        "kld-from",
			In:          "query",
			Required:    false,
		},
		SimpleSchema: spec.SimpleSchema{
			Type: "string",
		},
	}
	params["valueParam"] = spec.Parameter{
		ParamProps: spec.ParamProps{
			Description:     "Value to send with the transaction - 'x-kaleido-value' header can also be used",
			Name:            "kld-value",
			In:              "query",
			Required:        false,
			AllowEmptyValue: true,
		},
		SimpleSchema: spec.SimpleSchema{
			Type: "integer",
		},
	}
	params["gasParam"] = spec.Parameter{
		ParamProps: spec.ParamProps{
			Description:     "Gas to send with the transaction (auto-calculated if not set) - 'x-kaleido-gas' header can also be used",
			Name:            "kld-gas",
			In:              "query",
			Required:        false,
			AllowEmptyValue: true,
		},
		SimpleSchema: spec.SimpleSchema{
			Type: "integer",
		},
	}
	params["gaspriceParam"] = spec.Parameter{
		ParamProps: spec.ParamProps{
			Description:     "Gas Price offered - 'x-kaleido-gasprice' header can also be used",
			Name:            "kld-gasprice",
			In:              "query",
			Required:        false,
			AllowEmptyValue: true,
		},
		SimpleSchema: spec.SimpleSchema{
			Type: "integer",
		},
	}
	params["syncParam"] = spec.Parameter{
		ParamProps: spec.ParamProps{
			Description:     "Block the HTTP request until the tx is mined (does not store the receipt) - 'x-kaleido-sync' header can also be used",
			Name:            "kld-sync",
			In:              "query",
			Required:        false,
			AllowEmptyValue: true,
		},
		SimpleSchema: spec.SimpleSchema{
			Type:    "boolean",
			Default: true,
		},
	}
	params["callParam"] = spec.Parameter{
		ParamProps: spec.ParamProps{
			Description:     "Perform a read-only call with the same parameters that would be used to invoke, and return result - 'x-kaleido-call' header can also be used",
			Name:            "kld-call",
			In:              "query",
			Required:        false,
			AllowEmptyValue: true,
		},
		SimpleSchema: spec.SimpleSchema{
			Type: "boolean",
		},
	}
	return params
}

func (c *ABI2Swagger) addCommonParams(op *spec.Operation, isPOST bool, isConstructor bool) {
	fromParam, _ := spec.NewRef("#/parameters/fromParam")
	valueParam, _ := spec.NewRef("#/parameters/valueParam")
	gasParam, _ := spec.NewRef("#/parameters/gasParam")
	gaspriceParam, _ := spec.NewRef("#/parameters/gaspriceParam")
	syncParam, _ := spec.NewRef("#/parameters/syncParam")
	callParam, _ := spec.NewRef("#/parameters/callParam")
	op.Parameters = append(op.Parameters, spec.Parameter{
		Refable: spec.Refable{
			Ref: fromParam,
		},
	})
	op.Parameters = append(op.Parameters, spec.Parameter{
		Refable: spec.Refable{
			Ref: valueParam,
		},
	})
	op.Parameters = append(op.Parameters, spec.Parameter{
		Refable: spec.Refable{
			Ref: gasParam,
		},
	})
	op.Parameters = append(op.Parameters, spec.Parameter{
		Refable: spec.Refable{
			Ref: gaspriceParam,
		},
	})
	if isPOST {
		op.Parameters = append(op.Parameters, spec.Parameter{
			Refable: spec.Refable{
				Ref: syncParam,
			},
		})
		op.Parameters = append(op.Parameters, spec.Parameter{
			Refable: spec.Refable{
				Ref: callParam,
			},
		})
	}
}

func (c *ABI2Swagger) buildGETPath(outputSchema string, inst bool, method abi.Method, methodSig string, devdocs gjson.Result) *spec.Operation {
	parameters := make([]spec.Parameter, 0, len(method.Inputs)+1)
	if !inst {
		parameters = append(parameters, spec.Parameter{
			ParamProps: spec.ParamProps{
				Description: "The contract address",
				Name:        "address",
				In:          "path",
				Required:    true,
			},
			SimpleSchema: spec.SimpleSchema{
				Type: "string",
			},
		})
	}
	for _, input := range method.Inputs {
		desc := devdocs.Get("params." + input.Name).String()
		varDetails := desc
		if varDetails != "" {
			varDetails = ": " + desc
		}
		parameters = append(parameters, spec.Parameter{
			ParamProps: spec.ParamProps{
				Name:        input.Name,
				In:          "query",
				Description: input.Type.String() + varDetails,
				Required:    true,
			},
			SimpleSchema: spec.SimpleSchema{
				Type: "string",
			},
		})
	}
	op := &spec.Operation{
		OperationProps: spec.OperationProps{
			Summary:     methodSig,
			Description: devdocs.Get("details").String(),
			Produces:    []string{"application/json"},
			Responses:   c.buildResponses(outputSchema, method, devdocs),
			Parameters:  parameters,
		},
	}
	c.addCommonParams(op, false, false)
	return op
}

func (c *ABI2Swagger) buildPOSTPath(inputSchema, outputSchema string, inst, constructor bool, method abi.Method, methodSig string, devdocs gjson.Result) *spec.Operation {
	parameters := make([]spec.Parameter, 0, 2)
	if !inst && !constructor {
		parameters = append(parameters, spec.Parameter{
			ParamProps: spec.ParamProps{
				Description: "The contract address",
				Name:        "address",
				In:          "path",
				Required:    true,
			},
			SimpleSchema: spec.SimpleSchema{
				Type: "string",
			},
		})
	}
	ref, _ := jsonreference.New("#/definitions/" + inputSchema)
	parameters = append(parameters, spec.Parameter{
		ParamProps: spec.ParamProps{
			Name:     "body",
			In:       "body",
			Required: true,
			Schema: &spec.Schema{
				SchemaProps: spec.SchemaProps{
					Ref: spec.Ref{
						Ref: ref,
					},
				},
			},
		},
	})
	op := &spec.Operation{
		OperationProps: spec.OperationProps{
			Summary:     methodSig,
			Description: devdocs.Get("details").String(),
			Consumes:    []string{"application/json", "application/x-yaml"},
			Produces:    []string{"application/json"},
			Responses:   c.buildResponses(outputSchema, method, devdocs),
			Parameters:  parameters,
		},
	}
	c.addCommonParams(op, true, constructor)
	return op
}

func (c *ABI2Swagger) buildResponses(outputSchema string, method abi.Method, devdocs gjson.Result) *spec.Responses {
	errRef, _ := jsonreference.New("#/definitions/error")
	errorResponse := spec.Response{
		ResponseProps: spec.ResponseProps{
			Description: "error",
			Schema: &spec.Schema{
				SchemaProps: spec.SchemaProps{
					Ref: spec.Ref{
						Ref: errRef,
					},
				},
			},
		},
	}
	outputRef, _ := jsonreference.New("#/definitions/" + outputSchema)
	desc := devdocs.Get("return").String()
	if desc == "" {
		desc = "successful response"
	}
	return &spec.Responses{
		ResponsesProps: spec.ResponsesProps{
			StatusCodeResponses: map[int]spec.Response{
				200: spec.Response{
					ResponseProps: spec.ResponseProps{
						Description: desc,
						Schema: &spec.Schema{
							SchemaProps: spec.SchemaProps{
								Ref: spec.Ref{
									Ref: outputRef,
								},
							},
						},
					},
				},
			},
			Default: &errorResponse,
		},
	}
}

func (c *ABI2Swagger) buildArgumentsDefinition(defs map[string]spec.Schema, name string, args abi.Arguments, isReturn bool, devdocs gjson.Result) {

	s := spec.Schema{
		SchemaProps: spec.SchemaProps{
			Properties: make(map[string]spec.Schema),
		},
	}
	defs[name] = s

	for idx, arg := range args {
		argName := arg.Name
		if argName == "" {
			argName = "output"
			if idx != 0 {
				argName += strconv.Itoa(idx)
			}
		}
		argDocs := devdocs.Get("params." + arg.Name)
		s.Properties[argName] = c.mapArgToSchema(arg, isReturn, argDocs.String())
	}

}

func (c *ABI2Swagger) mapArgToSchema(arg abi.Argument, isReturn bool, desc string) spec.Schema {

	varDetails := desc
	if varDetails != "" {
		varDetails = ": " + desc
	}

	s := spec.Schema{
		SchemaProps: spec.SchemaProps{
			Description: arg.Type.String() + varDetails,
			Type:        []string{"string"},
		},
	}
	c.mapTypeToSchema(&s, arg.Type, isReturn)

	return s
}

func (c *ABI2Swagger) mapTypeToSchema(s *spec.Schema, t abi.Type, isReturn bool) {

	switch t.T {
	case abi.IntTy, abi.UintTy:
		s.Type = []string{"string"}
		s.Pattern = "^-?[0-9]+$"
		// We would like to indicate we support numbers in this field, but neither
		// type arrays or oneOf seem to work with the tooling
		break
	case abi.BoolTy:
		s.Type = []string{"boolean"}
		break
	case abi.AddressTy:
		s.Type = []string{"string"}
		s.Pattern = "^(0x)?[a-fA-F0-9]{40}$"
		break
	case abi.StringTy:
		s.Type = []string{"string"}
		break
	case abi.BytesTy:
		s.Type = []string{"string"}
		s.Pattern = "^(0x)?[a-fA-F0-9]+$"
		break
	case abi.FixedBytesTy:
		s.Type = []string{"string"}
		s.Pattern = "^(0x)?[a-fA-F0-9]{" + strconv.Itoa(t.Size*2) + "}$"
		break
	case abi.SliceTy, abi.ArrayTy:
		s.Type = []string{"array"}
		s.Items = &spec.SchemaOrArray{}
		s.Items.Schema = &spec.Schema{}
		c.mapTypeToSchema(s.Items.Schema, *t.Elem, isReturn)
		break
	}

}