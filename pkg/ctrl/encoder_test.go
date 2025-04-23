// Copyright 2025 Hedgehog
// SPDX-License-Identifier: Apache-2.0

package ctrl

import (
	"fmt"
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEncodeVPCID(t *testing.T) {
	for _, tt := range []struct {
		id       uint32
		expected string
		err      bool
	}{
		{0, "00000", false},
		{1, "00001", false},
		{2, "00002", false},
		{9, "00009", false},
		{10, "0000a", false},
		{11, "0000b", false},
		{12, "0000c", false},
		{33, "0000x", false},
		{34, "0000y", false},
		{35, "0000z", false},
		{36, "0000A", false},
		{37, "0000B", false},
		{38, "0000C", false},
		{59, "0000X", false},
		{60, "0000Y", false},
		{61, "0000Z", false},
		{62, "00010", false},
		{63, "00011", false},
		{64, "00012", false},
		{3843, "000ZZ", false},
		{3844, "00100", false},
		{916132831, "ZZZZZ", false},
		{916132832, "", true},
		{math.MaxUint32, "", true},
	} {
		t.Run(fmt.Sprintf("id-%d", tt.id), func(t *testing.T) {
			got, err := VPCID.Encode(tt.id)
			if tt.err {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}

			require.Equal(t, tt.expected, got)
		})
	}
}
