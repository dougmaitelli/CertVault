package config

import (
	"encoding"
	"fmt"
	"os"
	"reflect"
	"strconv"
	"strings"
)

// Application environment variable names.
const (
	EnvConfigFile = "CERTVAULT_CONFIG"
	EnvUIDir      = "CERTVAULT_UI_DIR"
	EnvEventJSON  = "CERTVAULT_EVENT_JSON"
)

// EnvFileSuffix marks a provider environment variable whose value is read
// from a mounted file.
const EnvFileSuffix = "_FILE"

var textUnmarshalerType = reflect.TypeFor[encoding.TextUnmarshaler]()

func applyEnv(c *Config) error {
	return applyEnvironmentSchema(reflect.ValueOf(c).Elem())
}

func applyEnvironmentSchema(value reflect.Value) error {
	valueType := value.Type()

	for index := range value.NumField() {
		field := value.Field(index)
		definition := valueType.Field(index)

		if definition.Anonymous || !field.CanSet() {
			continue
		}

		if field.Kind() == reflect.Struct && definition.Tag.Get("env") == "" {
			if err := applyEnvironmentSchema(field); err != nil {
				return err
			}

			continue
		}

		if field.Kind() == reflect.Pointer && field.Type().Elem().Kind() == reflect.Struct {
			if field.IsNil() && hasEnvironmentOverride(field.Type().Elem()) {
				field.Set(reflect.New(field.Type().Elem()))
			}

			if !field.IsNil() {
				if err := applyEnvironmentSchema(field.Elem()); err != nil {
					return err
				}
			}

			continue
		}

		environmentName := definition.Tag.Get("env")
		if definition.Tag.Get("file") == "true" {
			if err := applyFileEnvironment(value, field, definition, environmentName); err != nil {
				return err
			}

			continue
		}

		raw := os.Getenv(environmentName)
		if raw == "" {
			raw = definition.Tag.Get("default")
			if raw == "" || !field.IsZero() {
				continue
			}
		}

		if err := decodeConfigField(field, raw); err != nil {
			return fmt.Errorf("%s: %w", environmentName, err)
		}
	}

	return nil
}

func hasEnvironmentOverride(valueType reflect.Type) bool {
	for index := range valueType.NumField() {
		definition := valueType.Field(index)
		if environmentName := definition.Tag.Get("env"); environmentName != "" {
			if os.Getenv(environmentName) != "" ||
				definition.Tag.Get("file") == "true" && os.Getenv(environmentName+EnvFileSuffix) != "" {
				return true
			}
		}

		fieldType := definition.Type
		if fieldType.Kind() == reflect.Pointer {
			fieldType = fieldType.Elem()
		}

		if fieldType.Kind() == reflect.Struct && hasEnvironmentOverride(fieldType) {
			return true
		}
	}

	return false
}

func decodeConfigField(field reflect.Value, raw string) error {
	if field.CanAddr() && field.Addr().Type().Implements(textUnmarshalerType) {
		unmarshaler, ok := field.Addr().Interface().(encoding.TextUnmarshaler)
		if !ok {
			return fmt.Errorf("type %s does not implement encoding.TextUnmarshaler", field.Type())
		}

		err := unmarshaler.UnmarshalText([]byte(raw))

		return err
	}

	if field.Kind() == reflect.Pointer {
		value := reflect.New(field.Type().Elem())
		if err := decodeConfigField(value.Elem(), raw); err != nil {
			return err
		}

		field.Set(value)

		return nil
	}

	switch field.Kind() {
	case reflect.String:
		field.SetString(raw)
	case reflect.Bool:
		value, err := strconv.ParseBool(raw)
		if err != nil {
			return err
		}

		field.SetBool(value)
	case reflect.Slice:
		if field.Type().Elem().Kind() != reflect.String {
			return fmt.Errorf("unsupported slice type %s", field.Type())
		}

		values := splitCommaSeparated(raw)

		result := reflect.MakeSlice(field.Type(), len(values), len(values))
		for index, value := range values {
			result.Index(index).SetString(value)
		}

		field.Set(result)
	default:
		return fmt.Errorf("unsupported type %s", field.Type())
	}

	return nil
}

func applyFileEnvironment(
	parent reflect.Value,
	field reflect.Value,
	definition reflect.StructField,
	environmentName string,
) error {
	fileEnvironmentName := environmentName + EnvFileSuffix
	directValue := os.Getenv(environmentName)

	fileValue := os.Getenv(fileEnvironmentName)
	if directValue != "" && fileValue != "" {
		return fmt.Errorf("%s and %s cannot both be set", environmentName, fileEnvironmentName)
	}

	fileField := parent.FieldByName(definition.Name + "File")
	if fileField.IsValid() && (!fileField.CanSet() || fileField.Kind() != reflect.String) {
		return fmt.Errorf("%s: companion %sFile must be a settable string field", environmentName, definition.Name)
	}

	if directValue != "" {
		if err := decodeConfigField(field, directValue); err != nil {
			return fmt.Errorf("%s: %w", environmentName, err)
		}

		if fileField.IsValid() {
			fileField.SetZero()
		}

		return nil
	}

	filePath := fileValue
	if filePath == "" && fileField.IsValid() {
		filePath = fileField.String()
	}

	if filePath != "" {
		contents, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("%s: %w", fileEnvironmentName, err)
		}

		if err := decodeConfigField(field, strings.TrimSpace(string(contents))); err != nil {
			return fmt.Errorf("%s: %w", fileEnvironmentName, err)
		}

		if fileField.IsValid() {
			fileField.SetZero()
		}
	}

	return nil
}

func splitCommaSeparated(value string) []string {
	parts := strings.Split(value, ",")

	values := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			values = append(values, trimmed)
		}
	}

	return values
}
