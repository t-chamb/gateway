// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package protoyaml

import (
	"errors"
	"fmt"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"sigs.k8s.io/yaml"
)

var ErrEncoding = errors.New("encoding error")

func MarshalYAML(m proto.Message) ([]byte, error) {
	data, err := protojson.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("%w: marshal json: %w", ErrEncoding, err)
	}

	yamlData, err := yaml.JSONToYAML(data)
	if err != nil {
		return nil, fmt.Errorf("%w: convert json to yaml: %w", ErrEncoding, err)
	}

	return yamlData, nil
}

func UnmarshalYAML(data []byte, m proto.Message) error {
	jsonData, err := yaml.YAMLToJSONStrict(data)
	if err != nil {
		return fmt.Errorf("%w: convert yaml to json: %w", ErrEncoding, err)
	}

	if err := protojson.Unmarshal(jsonData, m); err != nil {
		return fmt.Errorf("%w: unmarshal json: %w", ErrEncoding, err)
	}

	return nil
}
