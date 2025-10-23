package json

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"

	"github.com/Velocidex/ordereddict"
	"github.com/go-json-experiment/json/jsontext"
	"github.com/kaptinlin/jsonschema"
	"www.velocidex.com/golang/velociraptor/json"
	"www.velocidex.com/golang/velociraptor/utils"
)

type ValidationOptions struct {
	// Allows fetching http schemas using a custom http client.
	Client utils.HTTPClient
}

func ParseJsonToObjectWithSchema(
	json_data string, schemas []string,
	options ValidationOptions) (res *ordereddict.Dict, errs []error) {

	result := ordereddict.NewDict()
	err := json.Unmarshal([]byte(json_data), &result)
	if err != nil {
		return nil, []error{utils.Wrap(utils.InvalidArgError,
			"While parsing %v: %v", utils.Elide(json_data, 50), err)}
	}

	intermediate, schema, errs := ParseJsonToMapWithSchema(
		json_data, schemas, options)
	if len(errs) > 0 {
		return nil, errs
	}

	// Populate defaults
	PopulateDefaults(result, intermediate, schema)

	return result, nil
}

func ParseJsonToMapWithSchema(
	json_string string, schemas []string,
	options ValidationOptions) (
	res map[string]interface{},
	schema *jsonschema.Schema, errs []error) {

	json_data := []byte(json_string)

	if len(schemas) == 0 {
		schemas = []string{"{}"}
	}

	compiler := jsonschema.NewCompiler()
	if options.Client != nil {
		allowURLs := func(url string) (io.ReadCloser, error) {
			return HTTPLoader(url, options)
		}

		compiler.RegisterLoader("http", allowURLs)
		compiler.RegisterLoader("https", allowURLs)

	} else {
		// jsonschema by default installs handlers but this a security
		// risk so we turn them off.
		compiler.RegisterLoader("http", rejectURL)
		compiler.RegisterLoader("https", rejectURL)
		compiler.RegisterLoader("file", rejectURL)
	}

	schema_data := []byte(schemas[0])
	schema, err := compiler.Compile(schema_data)
	if err != nil {
		return nil, nil, []error{DecorateError(schema_data, err)}
	}

	// Validate against the schema
	validation_res := schema.Validate(json_data)
	if !validation_res.IsValid() && len(validation_res.Errors) > 0 {
		var errors []error
		for _, details := range validation_res.Details {
			for _, v := range details.Errors {
				description := fmt.Sprintf("<red>%v</>", details.SchemaLocation)
				description_any := details.Annotations["description"]
				description_ptr, ok := description_any.(*string)
				if ok && description_ptr != nil {
					description = fmt.Sprintf("<red>%v</> (%v)", details.SchemaLocation,
						*description_ptr)
				}

				errors = append(errors,
					fmt.Errorf("%v: %w", description, v))
			}
		}

		return nil, schema, errors
	}

	// Parse into a map with defaults
	intermediate := make(map[string]interface{})
	err = schema.Unmarshal(&intermediate, json_data)
	if err != nil {
		return nil, schema, []error{utils.Wrap(utils.InvalidArgError,
			"While parsing %v: %v", utils.Elide(json_string, 50), err)}
	}

	return intermediate, schema, nil
}

func HTTPLoader(url string, opts ValidationOptions) (io.ReadCloser, error) {
	req, err := http.NewRequestWithContext(context.Background(), "GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := opts.Client.Do(req)
	if err != nil {
		return nil, jsonschema.ErrNetworkFetch
	}

	if resp.StatusCode != http.StatusOK {
		err = resp.Body.Close()
		if err != nil {
			return nil, err
		}
		return nil, jsonschema.ErrInvalidStatusCode
	}

	return resp.Body, nil
}

func rejectURL(url string) (io.ReadCloser, error) {
	return nil, fmt.Errorf("jsonschema: %w while getting %v",
		utils.PermissionDenied, url)
}

func PopulateDefaults(dest *ordereddict.Dict,
	src map[string]interface{}, schema *jsonschema.Schema) {
	for name, prop := range *schema.Properties {
		if prop.Default == nil {
			continue
		}

		// The ordered dict already has it.
		_, pres := dest.Get(name)
		if pres {
			continue
		}

		default_value, pres := src[name]
		if pres {
			default_obj, ok := default_value.(map[string]interface{})
			if ok {
				new_dict := ordereddict.NewDict()
				PopulateDefaults(new_dict, default_obj, prop)
				default_value = new_dict
			}

			dest.Set(name, default_value)
		}
	}
}

func cap(lower, upper, i int64) int64 {
	if i < lower {
		i = lower
	}

	if i > upper {
		i = upper
	}

	return i
}

var cleanupJSON = regexp.MustCompile("\n +")

func DecorateError(json_data []byte, err error) error {
	syntactic_err, ok := err.(*jsontext.SyntacticError)
	if !ok {
		return err
	}

	length := int64(len(json_data))
	start := cap(0, length, syntactic_err.ByteOffset-50)
	end := cap(0, length, syntactic_err.ByteOffset+50)
	middle := cap(0, length, syntactic_err.ByteOffset)

	return fmt.Errorf("%w: Context '%v ----> %v'", err,
		cleanupJSON.ReplaceAllString(
			string(json_data[start:middle]), ""),
		cleanupJSON.ReplaceAllString(
			string(json_data[middle:end]), ""),
	)
}
