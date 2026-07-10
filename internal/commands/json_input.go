package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"strings"
	"unicode/utf8"

	clierrors "github.com/inscoder/xero-cli/internal/errors"
)

const maxJSONInputBytes int64 = 1 << 20

// decodeJSONInput reads one strict JSON object from path into destination.
// A path of "-" reads stdin; callers must pass the terminal state explicitly
// so validation can happen before runtime or network construction.
func decodeJSONInput(path string, stdin io.Reader, stdinIsTerminal bool, destination any) error {
	if strings.TrimSpace(path) == "" {
		return clierrors.New(clierrors.KindValidation, "--file must not be empty")
	}

	var reader io.Reader
	var file *os.File
	if path == "-" {
		if stdinIsTerminal {
			return clierrors.New(clierrors.KindValidation, "--file - requires piped stdin; interactive stdin is not supported")
		}
		if stdin == nil {
			return clierrors.New(clierrors.KindValidation, "--file - requires stdin")
		}
		reader = stdin
	} else {
		// Stat before opening so named pipes and device files are rejected
		// without a potentially blocking read-only open. Fstat below remains
		// authoritative for the handle after open returns.
		pathInfo, err := os.Stat(path)
		if err != nil {
			return clierrors.New(clierrors.KindValidation, "JSON input file is missing or unreadable")
		}
		if !pathInfo.Mode().IsRegular() {
			return clierrors.New(clierrors.KindValidation, "JSON input must be a regular file")
		}
		if pathInfo.Size() > maxJSONInputBytes {
			return clierrors.New(clierrors.KindValidation, "JSON input exceeds the 1 MiB limit")
		}

		file, err = os.Open(path)
		if err != nil {
			return clierrors.New(clierrors.KindValidation, "JSON input file is missing or unreadable")
		}
		defer file.Close()

		info, err := file.Stat()
		if err != nil {
			return clierrors.New(clierrors.KindInternal, "inspect JSON input file")
		}
		if !info.Mode().IsRegular() {
			return clierrors.New(clierrors.KindValidation, "JSON input must be a regular file")
		}
		if info.Size() > maxJSONInputBytes {
			return clierrors.New(clierrors.KindValidation, "JSON input exceeds the 1 MiB limit")
		}
		reader = file
	}

	data, err := io.ReadAll(io.LimitReader(reader, maxJSONInputBytes+1))
	if err != nil {
		if path == "-" {
			return clierrors.Wrap(clierrors.KindInternal, "read JSON input", err)
		}
		return clierrors.New(clierrors.KindInternal, "read JSON input")
	}
	if int64(len(data)) > maxJSONInputBytes {
		return clierrors.New(clierrors.KindValidation, "JSON input exceeds the 1 MiB limit")
	}
	if bytes.HasPrefix(data, []byte{0xef, 0xbb, 0xbf}) {
		return clierrors.New(clierrors.KindValidation, "JSON input must not contain a UTF-8 BOM")
	}
	if !utf8.Valid(data) {
		return clierrors.New(clierrors.KindValidation, "JSON input must be valid UTF-8")
	}
	if err := validateSingleJSONObject(data); err != nil {
		return err
	}
	if err := validateExactJSONFields(data, destination); err != nil {
		return err
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(destination); err != nil {
		return clierrors.Wrap(clierrors.KindValidation, "decode JSON input", err)
	}
	return nil
}

func validateExactJSONFields(data []byte, destination any) error {
	destinationType := reflect.TypeOf(destination)
	if destinationType == nil || destinationType.Kind() != reflect.Pointer {
		return clierrors.New(clierrors.KindInternal, "JSON input destination must be a non-nil pointer")
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var document any
	if err := decoder.Decode(&document); err != nil {
		return clierrors.Wrap(clierrors.KindValidation, "decode JSON input fields", err)
	}
	return validateJSONFields(document, destinationType.Elem())
}

func validateJSONFields(value any, destinationType reflect.Type) error {
	for destinationType.Kind() == reflect.Pointer {
		destinationType = destinationType.Elem()
	}

	switch destinationType.Kind() {
	case reflect.Struct:
		object, ok := value.(map[string]any)
		if !ok {
			return nil
		}
		fields := make(map[string]reflect.Type)
		for index := 0; index < destinationType.NumField(); index++ {
			field := destinationType.Field(index)
			if field.PkgPath != "" {
				continue
			}
			name := strings.Split(field.Tag.Get("json"), ",")[0]
			if name == "-" {
				continue
			}
			if name == "" {
				name = field.Name
			}
			fields[name] = field.Type
		}
		for key, child := range object {
			fieldType, exists := fields[key]
			if !exists {
				return clierrors.New(clierrors.KindValidation, fmt.Sprintf("decode JSON input: json: unknown field %q", key))
			}
			if err := validateJSONFields(child, fieldType); err != nil {
				return err
			}
		}
	case reflect.Slice, reflect.Array:
		array, ok := value.([]any)
		if !ok {
			return nil
		}
		for _, child := range array {
			if err := validateJSONFields(child, destinationType.Elem()); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateSingleJSONObject(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()

	token, err := decoder.Token()
	if err != nil {
		if err == io.EOF {
			return clierrors.New(clierrors.KindValidation, "JSON input must contain one object")
		}
		return clierrors.Wrap(clierrors.KindValidation, "decode JSON input", err)
	}
	opening, ok := token.(json.Delim)
	if !ok || opening != '{' {
		return clierrors.New(clierrors.KindValidation, "JSON input must contain one object")
	}
	if err := validateJSONObjectTokens(decoder); err != nil {
		return err
	}

	if _, err := decoder.Token(); err == nil {
		return clierrors.New(clierrors.KindValidation, "JSON input must contain exactly one object with no trailing values")
	} else if err != io.EOF {
		return clierrors.Wrap(clierrors.KindValidation, "decode trailing JSON input", err)
	}
	return nil
}

func validateJSONObjectTokens(decoder *json.Decoder) error {
	seen := make(map[string]struct{})
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return clierrors.Wrap(clierrors.KindValidation, "decode JSON object key", err)
		}
		key, ok := token.(string)
		if !ok {
			return clierrors.New(clierrors.KindValidation, "JSON object keys must be strings")
		}
		if _, exists := seen[key]; exists {
			return clierrors.New(clierrors.KindValidation, fmt.Sprintf("JSON input contains duplicate key %q", key))
		}
		seen[key] = struct{}{}

		value, err := decoder.Token()
		if err != nil {
			return clierrors.Wrap(clierrors.KindValidation, fmt.Sprintf("decode JSON value for %q", key), err)
		}
		if err := validateJSONValue(decoder, value); err != nil {
			return err
		}
	}
	if _, err := decoder.Token(); err != nil {
		return clierrors.Wrap(clierrors.KindValidation, "decode JSON object", err)
	}
	return nil
}

func validateJSONArrayTokens(decoder *json.Decoder) error {
	for decoder.More() {
		value, err := decoder.Token()
		if err != nil {
			return clierrors.Wrap(clierrors.KindValidation, "decode JSON array value", err)
		}
		if err := validateJSONValue(decoder, value); err != nil {
			return err
		}
	}
	if _, err := decoder.Token(); err != nil {
		return clierrors.Wrap(clierrors.KindValidation, "decode JSON array", err)
	}
	return nil
}

func validateJSONValue(decoder *json.Decoder, value json.Token) error {
	if value == nil {
		return clierrors.New(clierrors.KindValidation, "JSON input must not contain null values")
	}
	delimiter, ok := value.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		return validateJSONObjectTokens(decoder)
	case '[':
		return validateJSONArrayTokens(decoder)
	default:
		return clierrors.New(clierrors.KindValidation, "JSON input contains an unexpected closing delimiter")
	}
}
