// SPDX-License-Identifier: AGPL-3.0-only

//go:build integration

package rawstore

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// S3Image is the S3-compatible server used by integration tests (pinned).
const S3Image = "versity/versitygw:v1.8.0@sha256:30292fc2eeacc67a36993b01f7a7a5e3361a19cced0e80c1d71cfa2a4b0a2499"

const (
	testAccessKey = "ohstestaccess"
	testSecretKey = "ohstestsecret-0123456789"
)

func TestS3(t *testing.T) {
	ctx := context.Background()
	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        S3Image,
			ExposedPorts: []string{"7070/tcp"},
			Env: map[string]string{
				"ROOT_ACCESS_KEY": testAccessKey,
				"ROOT_SECRET_KEY": testSecretKey,
			},
			Tmpfs:      map[string]string{"/data": "rw"},
			Cmd:        []string{"--port", ":7070", "posix", "/data"},
			WaitingFor: wait.ForListeningPort("7070/tcp").WithStartupTimeout(60 * time.Second),
		},
		Started: true,
	})
	testcontainers.CleanupContainer(t, c)
	require.NoError(t, err)

	host, err := c.Host(ctx)
	require.NoError(t, err)
	port, err := c.MappedPort(ctx, "7070/tcp")
	require.NoError(t, err)
	endpoint := fmt.Sprintf("http://%s:%s", host, port.Port())

	t.Setenv("AWS_ACCESS_KEY_ID", testAccessKey)
	t.Setenv("AWS_SECRET_ACCESS_KEY", testSecretKey)
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")

	st, err := Open(fmt.Sprintf("s3://ohs-raw?endpoint=%s&region=us-east-1&path_style=true", endpoint))
	require.NoError(t, err)
	s := st.(*s3Store)
	_, err = s.client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String("ohs-raw")})
	require.NoError(t, err)

	RunConformance(t, s)

	t.Run("many keys cross list pages and delete batches", func(t *testing.T) {
		for i := 0; i < 1005; i++ {
			require.NoError(t, s.Put(ctx, fmt.Sprintf("raw/t/big/%05d", i), strings.NewReader("v")))
		}
		keys, err := s.List(ctx, "raw/t/big/")
		require.NoError(t, err)
		require.Len(t, keys, 1005)
		require.NoError(t, s.DeletePrefix(ctx, "raw/t/big/"))
		keys, err = s.List(ctx, "raw/t/big/")
		require.NoError(t, err)
		require.Empty(t, keys)
	})
}
