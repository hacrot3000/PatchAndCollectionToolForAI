package dbmongo

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type scriptResponse struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

func buildScript(config Config, operationBody string) (string, error) {
	uri, err := config.URI()
	if err != nil {
		return "", err
	}
	uriLiteral, err := javascriptString(uri)
	if err != nil {
		return "", err
	}
	markerLiteral, err := javascriptString(resultMarker)
	if err != nil {
		return "", err
	}
	operationBody = strings.TrimSpace(operationBody)
	if operationBody == "" {
		return "", errors.New("mongosh operation body is required")
	}
	script := "(function(){\n" +
		"try {\n" +
		"  const __taskdeckDb = connect(" + uriLiteral + ");\n" +
		"  const __taskdeckResult = (function(){\n" +
		operationBody + "\n" +
		"  })();\n" +
		"  print(" + markerLiteral + " + EJSON.stringify({ok:true,result:__taskdeckResult},{relaxed:true}));\n" +
		"} catch (__taskdeckError) {\n" +
		"  print(" + markerLiteral + " + EJSON.stringify({ok:false,error:String(__taskdeckError)},{relaxed:true}));\n" +
		"}\n" +
		"})();\n"
	return script, nil
}

func javascriptString(value string) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode JavaScript string: %w", err)
	}
	return string(data), nil
}

func javascriptJSON(value interface{}) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("encode JavaScript JSON: %w", err)
	}
	literal, err := javascriptString(string(data))
	if err != nil {
		return "", err
	}
	return "JSON.parse(" + literal + ")", nil
}

func parseScriptResult(output []byte, secret string, target interface{}) error {
	scanner := bufio.NewScanner(bytes.NewReader(output))
	scanner.Buffer(make([]byte, 64<<10), maxShellStdoutBytes)
	var payload string
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, resultMarker) {
			return errors.New("mongosh emitted unexpected stdout")
		}
		if payload != "" {
			return errors.New("mongosh emitted multiple TaskDeck results")
		}
		payload = strings.TrimPrefix(line, resultMarker)
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read mongosh result: %w", err)
	}
	if payload == "" {
		return errors.New("mongosh emitted no TaskDeck result")
	}

	var response scriptResponse
	decoder := json.NewDecoder(strings.NewReader(payload))
	decoder.UseNumber()
	if err := decoder.Decode(&response); err != nil {
		return fmt.Errorf("decode mongosh result envelope: %w", err)
	}
	if !response.OK {
		message := strings.TrimSpace(response.Error)
		message = redactMongoSecret(message, secret)
		if message == "" {
			message = "MongoDB operation failed"
		}
		if len(message) > 4096 {
			message = message[:4096]
		}
		return errors.New(message)
	}
	if target == nil {
		return nil
	}
	if len(response.Result) == 0 {
		return errors.New("mongosh result payload is missing")
	}
	decoder = json.NewDecoder(bytes.NewReader(response.Result))
	decoder.UseNumber()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode mongosh operation result: %w", err)
	}
	return nil
}
