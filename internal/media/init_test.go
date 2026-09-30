package media

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"egeism/internal/config"
)

// Fake the S3 wire protocol, so these checks exercise the real SDK calls and
// catch accidentally publishing student solutions or issuing destructive I/O.
type bucketFixture struct {
	mu       sync.Mutex
	exists   map[string]bool
	policies map[string]string
	created  map[string]int
	fail     string
}

func newBucketFixture(t *testing.T) (*bucketFixture, config.Config) {
	t.Helper()
	f := &bucketFixture{exists: map[string]bool{}, policies: map[string]string{}, created: map[string]int{}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		bucket := strings.Trim(r.URL.Path, "/")
		if bucket != "task-media" && bucket != "task-media-solutions" {
			t.Errorf("unexpected bucket or object operation: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if r.Header.Get("Authorization") == "" {
			t.Error("storage initialization request was not authenticated")
		}
		operation := r.Method + " " + r.URL.RequestURI()
		if operation == f.fail {
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, `<Error><Code>AccessDenied</Code><Message>Access denied</Message></Error>`)
			return
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Query().Has("location"):
			w.Header().Set("Content-Type", "application/xml")
			_, _ = io.WriteString(w, `<LocationConstraint xmlns="http://s3.amazonaws.com/doc/2006-03-01/">us-east-1</LocationConstraint>`)
		case r.Method == http.MethodHead:
			if !f.exists[bucket] {
				w.WriteHeader(http.StatusNotFound)
			}
		case r.Method == http.MethodPut && r.URL.Query().Has("policy"):
			data, err := io.ReadAll(r.Body)
			if err != nil {
				t.Error(err)
			}
			f.policies[bucket] = string(data)
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodDelete && r.URL.Query().Has("policy"):
			delete(f.policies, bucket)
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodPut && r.URL.RawQuery == "":
			f.exists[bucket] = true
			f.created[bucket]++
		default:
			t.Errorf("unexpected S3 operation: %s", operation)
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	t.Cleanup(server.Close)
	return f, config.Config{MinIOEndpoint: strings.TrimPrefix(server.URL, "http://"),
		MinIOBucket: "task-media", MinIOAccessKey: "test-access", MinIOSecretKey: "test-secret"}
}

func TestInitBucketsCreatesAndReconcilesPolicies(t *testing.T) {
	f, cfg := newBucketFixture(t)
	for run := range 2 {
		if err := InitBuckets(context.Background(), cfg); err != nil {
			t.Fatal(err)
		}
		f.mu.Lock()
		for _, bucket := range []string{cfg.MinIOBucket, cfg.MinIOBucket + "-solutions"} {
			if !f.exists[bucket] || f.created[bucket] != 1 {
				t.Errorf("run %d bucket %s: exists=%v creates=%d", run, bucket, f.exists[bucket], f.created[bucket])
			}
		}
		if f.policies[cfg.MinIOBucket+"-solutions"] != "" {
			t.Error("student solutions still have an anonymous policy")
		}
		var document struct {
			Version   string
			Statement []struct {
				Effect    string
				Principal struct{ AWS []string }
				Action    []string
				Resource  []string
			}
		}
		if err := json.Unmarshal([]byte(f.policies[cfg.MinIOBucket]), &document); err != nil {
			t.Error(err)
		}
		foundDownload := false
		for _, statement := range document.Statement {
			if statement.Effect != "Allow" || len(statement.Principal.AWS) != 1 || statement.Principal.AWS[0] != "*" {
				t.Errorf("unexpected principal/effect: %+v", statement)
			}
			for _, action := range statement.Action {
				switch action {
				case "s3:GetObject":
					foundDownload = true
				case "s3:GetBucketLocation", "s3:ListBucket":
				default:
					t.Errorf("public policy grants non-read permission %s", action)
				}
			}
			for _, resource := range statement.Resource {
				if resource != "arn:aws:s3:::task-media" && resource != "arn:aws:s3:::task-media/*" {
					t.Errorf("public policy escapes task bucket: %s", resource)
				}
			}
		}
		if document.Version != "2012-10-17" || !foundDownload {
			t.Error("task download policy is missing or invalid")
		}
		// Simulate policy drift before the second deployment: it must be repaired
		// while existing buckets and their objects remain intact.
		f.policies[cfg.MinIOBucket] = `{"unwanted":"write access"}`
		f.policies[cfg.MinIOBucket+"-solutions"] = `{"unwanted":"public access"}`
		f.mu.Unlock()
	}
}

func TestInitBucketsPropagatesFailures(t *testing.T) {
	for _, operation := range []string{
		"PUT /task-media/",
		"DELETE /task-media-solutions/?policy=",
		"PUT /task-media/?policy=",
	} {
		t.Run(operation, func(t *testing.T) {
			f, cfg := newBucketFixture(t)
			f.fail = operation
			if err := InitBuckets(context.Background(), cfg); err == nil {
				t.Fatal("storage initialization succeeded after denied operation")
			}
			f.mu.Lock()
			defer f.mu.Unlock()
			if f.policies[cfg.MinIOBucket] != "" {
				t.Error("published task bucket despite incomplete initialization")
			}
		})
	}
}

func TestNewPreservesExistingTaskPolicy(t *testing.T) {
	f, cfg := newBucketFixture(t)
	f.policies[cfg.MinIOBucket] = "existing custom policy"
	if _, err := New(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.policies[cfg.MinIOBucket] != "existing custom policy" {
		t.Fatal("ordinary API startup changed the task bucket policy")
	}
}

func TestInitBucketsHonorsDeadline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	cfg := config.Config{MinIOEndpoint: strings.TrimPrefix(server.URL, "http://"),
		MinIOBucket: "task-media", MinIOAccessKey: "test-access", MinIOSecretKey: "test-secret"}
	started := time.Now()
	if err := InitBuckets(ctx, cfg); err == nil {
		t.Fatal("unresponsive storage did not fail")
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatal(fmt.Sprintf("storage deadline took %s", elapsed))
	}
}
