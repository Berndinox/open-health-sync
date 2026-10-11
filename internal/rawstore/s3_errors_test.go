// SPDX-License-Identifier: AGPL-3.0-only

package rawstore

import (
	"errors"
	"net/http"
	"testing"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"
	"github.com/stretchr/testify/require"
)

func httpErr(status int, code string) error {
	return &smithyhttp.ResponseError{
		Response: &smithyhttp.Response{Response: &http.Response{StatusCode: status}},
		Err:      &smithy.GenericAPIError{Code: code, Message: "m"},
	}
}

func TestS3ExistsMapping(t *testing.T) {
	require.True(t, isExistsErr(httpErr(412, "PreconditionFailed")))
	require.True(t, isExistsErr(httpErr(409, "ConditionalRequestConflict")))
	require.True(t, isExistsErr(&smithy.GenericAPIError{Code: "ConditionalRequestConflict"}))
	require.True(t, isExistsErr(&smithy.GenericAPIError{Code: "PreconditionFailed"}))
	require.False(t, isExistsErr(httpErr(500, "InternalError")))
	require.False(t, isExistsErr(httpErr(403, "AccessDenied")))
	require.False(t, isExistsErr(errors.New("boom")))
}

func TestS3NotFoundMapping(t *testing.T) {
	require.True(t, isNoSuchKey(&types.NoSuchKey{}))
	require.True(t, isNoSuchKey(httpErr(404, "NoSuchKey")))
	require.False(t, isNoSuchKey(httpErr(404, "NoSuchBucket")))
	require.False(t, isNoSuchKey(httpErr(404, "NotFound")))
	require.False(t, isNoSuchKey(errors.New("boom")))
}
