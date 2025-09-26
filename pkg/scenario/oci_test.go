package scenario

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_F_OCI(t *testing.T) {
	t.Parallel()

	var tests = map[string]struct {
		Ref          string
		ExpectedName string
		ExpectedDig  string
		ExpectErr    bool
	}{
		"named": {
			// For this case we use an image that will not be updated thus
			// we guarantee that "latest" image has indeed this corresponding hash.
			Ref:          "pandatix/license-lvl1",
			ExpectedName: "pandatix/license-lvl1",
			ExpectedDig:  "sha256:acaca41979983579c871d13d407f2167c7394605fc08f3f36eb7484f12dbaf62",
		},
		"named-tagged": {
			Ref:          "ctferio/chall-manager:v0.1.0-rc0",
			ExpectedName: "ctferio/chall-manager",
			ExpectedDig:  "sha256:272a63ea0a75b63f6dc6e34bce0b8591c9fc15549a1298b3ee9e2685a38bff7e",
		},
		"invalid digest named-tagged": {
			//nolint:lll
			Ref:          "ctferio/chall-manager:v0.1.0-rc0@sha256:db05b77564502a2db3409d0970810b8586b74d8fa40d0f234ca4a087d06c28d2", // actually the digest of v0.3.2
			ExpectedName: "ctferio/chall-manager",
			ExpectedDig:  "sha256:db05b77564502a2db3409d0970810b8586b74d8fa40d0f234ca4a087d06c28d2",
			ExpectErr:    false, // in this specific case, the canonical form prevails thus it is valid
		},
		"canonical": {
			//nolint:lll
			Ref:          "ctferio/chall-manager:v0.1.0-rc0@sha256:272a63ea0a75b63f6dc6e34bce0b8591c9fc15549a1298b3ee9e2685a38bff7e",
			ExpectedName: "ctferio/chall-manager",
			ExpectedDig:  "sha256:272a63ea0a75b63f6dc6e34bce0b8591c9fc15549a1298b3ee9e2685a38bff7e",
		},
		"registry named-taged": {
			Ref:       "localhost:5000/ctferio/chall-manager:v0.1.0-rc0",
			ExpectErr: true,
		},
	}

	for testname, tt := range tests {
		t.Run(testname, func(t *testing.T) {
			name, dig, err := resolve(t.Context(), tt.Ref, false, "", "")

			if tt.ExpectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)

				assert.Equal(t, tt.ExpectedName, name)
				assert.Equal(t, tt.ExpectedDig, dig)
			}
		})
	}
}
