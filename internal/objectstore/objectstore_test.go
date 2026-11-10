package objectstore_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/adirkuhn/moneyharpy/internal/objectstore"
	"github.com/adirkuhn/moneyharpy/internal/settings"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

type fixedSettings struct{ storage settings.Storage }

func (f fixedSettings) Instance(context.Context) (settings.Instance, error) {
	return settings.Instance{Storage: f.storage}, nil
}

// s3 is a SeaweedFS S3 server shared by the tests, standing in for any
// S3-compatible service. Each test uses its own bucket.
var s3 settings.Storage

func TestMain(m *testing.M) {
	ctx := context.Background()
	c, err := testcontainers.Run(ctx, "chrislusf/seaweedfs:4.52",
		testcontainers.WithCmd("server", "-s3", "-s3.port=8333", "-dir=/data", "-s3.config=/etc/seaweedfs/s3.json"),
		testcontainers.WithFiles(testcontainers.ContainerFile{HostFilePath: "testdata/s3.json", ContainerFilePath: "/etc/seaweedfs/s3.json", FileMode: 0o644}),
		testcontainers.WithExposedPorts("8333/tcp"),
		testcontainers.WithWaitStrategy(wait.ForHTTP("/healthz").WithPort("8333/tcp").WithStartupTimeout(time.Minute)),
	)
	if err != nil {
		panic(err)
	}
	host, err := c.PortEndpoint(ctx, "8333/tcp", "")
	if err != nil {
		panic(err)
	}
	s3 = settings.Storage{
		Enabled: true, Endpoint: "http://" + host, Region: "us-east-1",
		AccessKey: "s3-test", SecretKey: "s3-test-secret", PathStyle: true,
	}
	code := m.Run()
	_ = c.Terminate(ctx)
	os.Exit(code)
}

var buckets atomic.Int64

// startS3 returns settings for a new, empty bucket on the shared server.
func startS3(t *testing.T) settings.Storage {
	t.Helper()
	cfg := s3
	cfg.Bucket = fmt.Sprintf("bucket-%d", buckets.Add(1))
	if err := objectstore.New(fixedSettings{cfg}).Check(context.Background()); err != nil {
		t.Fatalf("prepare bucket: %v", err)
	}
	return cfg
}

func TestPutGetDelete(t *testing.T) {
	ctx := context.Background()
	store := objectstore.New(fixedSettings{startS3(t)})

	if err := store.Check(ctx); err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	data := []byte("\x89PNG fake image")
	if err := store.Put(ctx, "logos/a.png", "image/png", data); err != nil {
		t.Fatalf("Put() error = %v", err)
	}
	obj, err := store.Get(ctx, "logos/a.png")
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	got, _ := io.ReadAll(obj.Body)
	_ = obj.Body.Close()
	if !bytes.Equal(got, data) || obj.ContentType != "image/png" {
		t.Errorf("Get() = %q (%s), want %q (image/png)", got, obj.ContentType, data)
	}
	if err := store.Delete(ctx, "logos/a.png"); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if _, err := store.Get(ctx, "logos/a.png"); !errors.Is(err, objectstore.ErrNotFound) {
		t.Errorf("Get() after delete error = %v, want ErrNotFound", err)
	}
}

func TestCheckCreatesAMissingBucket(t *testing.T) {
	cfg := startS3(t)
	cfg.Bucket = "created-by-check"
	store := objectstore.New(fixedSettings{cfg})
	if err := store.Check(context.Background()); err != nil {
		t.Fatalf("Check() error = %v", err)
	}
	if err := store.Put(context.Background(), "a.txt", "text/plain", []byte("x")); err != nil {
		t.Errorf("Put() after Check() error = %v", err)
	}
}

func TestDisabledStorage(t *testing.T) {
	store := objectstore.New(fixedSettings{settings.Storage{}})
	if err := store.Put(context.Background(), "a", "image/png", []byte("x")); !errors.Is(err, objectstore.ErrDisabled) {
		t.Errorf("Put() error = %v, want ErrDisabled", err)
	}
}

func TestCheckReportsWrongCredentials(t *testing.T) {
	cfg := startS3(t)
	cfg.SecretKey = "wrong"
	if err := objectstore.New(fixedSettings{cfg}).Check(context.Background()); err == nil {
		t.Error("Check() with a wrong secret error = nil")
	}
}
