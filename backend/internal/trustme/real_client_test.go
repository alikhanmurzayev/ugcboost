package trustme

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRealClient_SendToSign(t *testing.T) {
	t.Parallel()

	t.Run("success parses response", func(t *testing.T) {
		t.Parallel()
		var receivedToken, receivedContentType string
		var receivedBody []byte
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedToken = r.Header.Get("Authorization")
			receivedContentType = r.Header.Get("Content-Type")
			receivedBody, _ = io.ReadAll(r.Body)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"status":"Ok","errorText":"",
				"data":{"url":"https://t.tct.kz/uploader/abc","document_id":"abc","fileName":"abc.pdf"}
			}`))
		}))
		defer srv.Close()

		client := NewRealClient(srv.URL, "tokenZ", srv.Client())
		got, err := client.SendToSign(context.Background(), SendToSignInput{
			PDFBase64:      "JVBERi0xLg==",
			AdditionalInfo: "ct-1",
			ContractName:   "Договор",
			Requisites: []Requisite{{
				FIO: "Иванов Иван", IINBIN: "880101300123", PhoneNumber: "+77071234567",
			}},
		})
		require.NoError(t, err)
		require.Equal(t, &SendToSignResult{
			DocumentID: "abc",
			ShortURL:   "https://t.tct.kz/uploader/abc",
			FileName:   "abc.pdf",
		}, got)
		require.Equal(t, "tokenZ", receivedToken)
		require.True(t, strings.HasPrefix(receivedContentType, "multipart/form-data"))
		require.Contains(t, string(receivedBody), `"FIO":"Иванов Иван"`)
		require.Contains(t, string(receivedBody), `"IIN_BIN":"880101300123"`)
		require.Contains(t, string(receivedBody), `"PhoneNumber":"+77071234567"`)
		require.Contains(t, string(receivedBody), `"AdditionalInfo":"ct-1"`)
		require.Contains(t, string(receivedBody), "JVBERi0xLg==")
	})

	t.Run("error status returns error", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"Error","errorText":"1208","data":""}`))
		}))
		defer srv.Close()

		client := NewRealClient(srv.URL, "tk", srv.Client())
		_, err := client.SendToSign(context.Background(), SendToSignInput{
			PDFBase64:      "JVBERi0xLg==",
			AdditionalInfo: "ct-2",
			ContractName:   "Контракт",
			Requisites: []Requisite{{
				FIO: "X", IINBIN: "1", PhoneNumber: "+77",
			}},
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "1208")
	})

	t.Run("non-200 returns error", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "boom", http.StatusServiceUnavailable)
		}))
		defer srv.Close()
		client := NewRealClient(srv.URL, "tk", srv.Client())
		_, err := client.SendToSign(context.Background(), SendToSignInput{
			PDFBase64: "x", AdditionalInfo: "ct-3", ContractName: "x",
			Requisites: []Requisite{{FIO: "x", IINBIN: "1", PhoneNumber: "+77"}},
		})
		require.Error(t, err)
		require.Contains(t, err.Error(), "503")
	})
}

func TestRealClient_SearchContractByAdditionalInfo(t *testing.T) {
	t.Parallel()

	t.Run("found returns first matching item", func(t *testing.T) {
		t.Parallel()
		var bodyJSON map[string]any
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			body, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(body, &bodyJSON)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"status":"Ok","errorText":"",
				"data":[{"id":"i1","shortUrl":"abc","contractStatus":2,"additionalInfo":"ct-1"}]
			}`))
		}))
		defer srv.Close()

		client := NewRealClient(srv.URL, "tk", srv.Client())
		got, err := client.SearchContractByAdditionalInfo(context.Background(), "ct-1")
		require.NoError(t, err)
		require.Equal(t, &SearchContractResult{
			DocumentID: "i1", ShortURL: "abc", ContractStatus: 2,
		}, got)
		require.NotNil(t, bodyJSON)
		require.Equal(t, "CreatedAt", bodyJSON["orderField"])
	})

	t.Run("empty data returns ErrTrustMeNotFound", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"Ok","errorText":"","data":[]}`))
		}))
		defer srv.Close()
		client := NewRealClient(srv.URL, "tk", srv.Client())
		_, err := client.SearchContractByAdditionalInfo(context.Background(), "ct-missing")
		require.True(t, errors.Is(err, ErrTrustMeNotFound))
	})

	t.Run("empty additionalInfo errors", func(t *testing.T) {
		t.Parallel()
		client := NewRealClient("http://localhost", "tk", nil)
		_, err := client.SearchContractByAdditionalInfo(context.Background(), "")
		require.Error(t, err)
	})
}

func TestRealClient_DownloadContractFile(t *testing.T) {
	t.Parallel()

	t.Run("success returns body", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			require.Equal(t, "/doc/DownloadContractFile/doc-xyz", r.URL.Path)
			require.Equal(t, "tk", r.Header.Get("Authorization"))
			_, _ = w.Write([]byte("PDF-bytes"))
		}))
		defer srv.Close()
		client := NewRealClient(srv.URL, "tk", srv.Client())
		body, err := client.DownloadContractFile(context.Background(), "doc-xyz")
		require.NoError(t, err)
		require.Equal(t, []byte("PDF-bytes"), body)
	})

	t.Run("empty id errors", func(t *testing.T) {
		t.Parallel()
		client := NewRealClient("http://localhost", "tk", nil)
		_, err := client.DownloadContractFile(context.Background(), "")
		require.Error(t, err)
	})

	t.Run("non-200 returns error", func(t *testing.T) {
		t.Parallel()
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "missing", http.StatusNotFound)
		}))
		defer srv.Close()
		client := NewRealClient(srv.URL, "tk", srv.Client())
		_, err := client.DownloadContractFile(context.Background(), "missing")
		require.Error(t, err)
		require.Contains(t, err.Error(), "404")
	})
}
