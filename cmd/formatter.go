package cmd

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/BurntSushi/toml"
	"gopkg.in/yaml.v3"

	"github.com/leaktk/leaktk/pkg/config"
	"github.com/leaktk/leaktk/pkg/logger"
	"github.com/leaktk/leaktk/pkg/proto"
)

// OutputFormat is the code(int) for each format
type OutputFormat int

const (
	// JSON displays the output in JSON format
	JSON OutputFormat = iota
	// HUMAN displays the outut in a way that's nice for humans to read
	HUMAN
	// TOML displays the output in TOML format
	TOML
	// YAML displays the output in YAML format
	YAML
	// CSV displays the output in CSV format
	CSV
)

// Formatter handles the output format for the response
type Formatter struct {
	format OutputFormat
}

// NewFormatter creates new formatter
func NewFormatter(cfg config.Formatter) (*Formatter, error) {
	format, err := getOutputFormat(cfg.Format)
	if err != nil {
		return nil, err
	}

	return &Formatter{format: format}, nil
}

func getOutputFormat(format string) (OutputFormat, error) {
	format = strings.ToUpper(format)
	switch format {
	case "JSON":
		return JSON, nil
	case "HUMAN":
		return HUMAN, nil
	case "TOML":
		return TOML, nil
	case "YAML":
		return YAML, nil
	case "CSV":
		return CSV, nil
	default:
		return JSON, fmt.Errorf("invalid output format option: format=%q", format)
	}
}

// Format renders a response structure to the set format as a string
func (f *Formatter) Format(r *proto.Response) string {
	switch f.format {
	case JSON:
		return formatJSON(r)
	case HUMAN:
		return formatHuman(r)
	case TOML:
		return formatToml(r)
	case YAML:
		return formatYaml(r)
	case CSV:
		return formatCsv(r)
	default:
		return formatJSON(r)
	}
}

func formatJSON(r *proto.Response) string {
	out, err := json.Marshal(r)
	if err != nil {
		logger.Error("could not marshal response: error=%q", err)
	}

	return string(out)
}

func formatHuman(r *proto.Response) string {
	headers, responses := flattenedResponse(r)

	for i, header := range headers {
		// Append a : to each label. Do it once here instead of every loop
		headers[i] = header + ":"
	}

	var out []string
	for _, response := range responses {
		for i, entry := range response {
			// Specifies width of 26 characters for labels
			out = append(out, fmt.Sprintf("%-26s%s", headers[i], entry))
		}
		out = append(out, "\n")
	}

	return strings.Join(out, "\n")
}

func formatToml(r *proto.Response) string {
	var buf bytes.Buffer

	if err := toml.NewEncoder(&buf).Encode(r); err != nil {
		logger.Error("could not marshal response: error=%q", err)
	}

	return buf.String()
}

func formatYaml(r *proto.Response) string {
	out, err := yaml.Marshal(r)
	if err != nil {
		logger.Error("could not marshal response: error=%q", err)
	}

	return string(out)
}

func formatCsv(r *proto.Response) string {
	headers, responses := flattenedResponse(r)

	var buf bytes.Buffer
	writer := csv.NewWriter(&buf)

	err := writer.Write(headers)
	if err != nil {
		logger.Error("could not write response: error=%q", err)
	}

	err = writer.WriteAll(responses)
	if err != nil {
		logger.Error("could not write response: error=%q", err)
	}

	return buf.String()
}

// flattenedResponse takes the response and returns the responsefields and a 2d list of responses
func flattenedResponse(response *proto.Response) ([]string, [][]string) {
	var flattened [][]string

	for _, result := range response.Results {
		flattened = append(flattened, []string{
			response.ID,
			response.RequestID,
			result.ID,
			result.Kind,
			result.Rule.ID,
			result.Rule.Description,
			flattenContact(result.Contact),
			result.Secret,
			result.Match,
			fmt.Sprintf("%f", result.Entropy),
			result.Date,
			result.Location.Version,
			result.Location.Path,
			fmt.Sprintf("L%dC%d-L%dC%d", result.Location.Start.Line,
				result.Location.Start.Column, result.Location.End.Line, result.Location.End.Column),
			strings.Join(result.Rule.Tags, ", "),
		})
	}

	return []string{"ID", "REQUEST.ID", "RESULT.ID", "RESULT.KIND", "RESULT.RULE.ID", "RESULT.RULE.DESCRIPTION",
		"RESULT.CONTACT", "RESULT.SECRET", "RESULT.MATCH", "RESULT.ENTROPY", "RESULT.DATE", "RESULT.LOCATION.VERSION",
		"RESULT.LOCATION.PATH", "RESULT.LOCATION.RANGE", "RESULT.RULE.TAGS"}, flattened
}

// flattenContact creates a single string with Contact information as "Name <Email>"
func flattenContact(contact proto.Contact) string {
	return fmt.Sprintf("%s <%s>", contact.Name, contact.Email)
}
