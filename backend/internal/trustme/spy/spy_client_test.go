package spy

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/internal/trustme"
)

func newClient(t *testing.T) (*Client, *SpyStore) {
	t.Helper()
	store := NewSpyStore()
	fixed := time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC)
	c := NewClient(store, func() time.Time { return fixed })
	return c, store
}

func TestSpyClient_SendToSign_Records(t *testing.T) {
	t.Parallel()

	c, store := newClient(t)
	in := trustme.SendToSignInput{
		PDFBase64:      "JVBERi0xLg==",
		AdditionalInfo: "ct-1",
		ContractName:   "Договор UGC #1",
		Requisites: []trustme.Requisite{
			{
				CompanyName: "Креатор",
				FIO:         "Иванов Иван Иванович",
				IINBIN:      "880101300123",
				PhoneNumber: "+77071234567",
			},
		},
	}

	got, err := c.SendToSign(context.Background(), in)
	require.NoError(t, err)
	require.NotEmpty(t, got.DocumentID)
	require.Equal(t, "https://test.trustme.kz/uploader/"+got.DocumentID, got.ShortURL)

	// determinism — same additionalInfo → same document_id
	got2, err := c.SendToSign(context.Background(), in)
	require.NoError(t, err)
	require.Equal(t, got.DocumentID, got2.DocumentID)

	records := store.List()
	require.Len(t, records, 2)
	require.Equal(t, "ct-1", records[0].AdditionalInfo)
	// PII полях больше не raw — security.md hard rule. Сравниваем
	// фингерпринты через ту же Fingerprint функцию.
	require.Equal(t, Fingerprint("Иванов Иван Иванович"), records[0].FIOFingerprint)
	require.Equal(t, Fingerprint("880101300123"), records[0].IINFingerprint)
	require.Equal(t, Fingerprint("+77071234567"), records[0].PhoneFingerprint)
	require.Equal(t, HashPDFBase64("JVBERi0xLg=="), records[0].PDFSha256)
	require.Empty(t, records[0].Err)
}

func TestFingerprint_Empty(t *testing.T) {
	t.Parallel()
	require.Equal(t, "", Fingerprint(""))
	require.Equal(t, Fingerprint("X"), Fingerprint("X"))
	require.NotEqual(t, Fingerprint("A"), Fingerprint("B"))
	require.Len(t, Fingerprint("X"), 16)
}

func TestSpyClient_SendToSign_FailNext(t *testing.T) {
	t.Parallel()

	c, store := newClient(t)
	store.RegisterFailNext("ct-1", "synthetic 502", 1)

	_, err := c.SendToSign(context.Background(), trustme.SendToSignInput{
		AdditionalInfo: "ct-1",
		Requisites: []trustme.Requisite{{
			FIO: "X", IINBIN: "1", PhoneNumber: "+77",
		}},
	})
	require.Error(t, err)
	require.Equal(t, "synthetic 502", err.Error())

	records := store.List()
	require.Len(t, records, 1)
	require.Equal(t, "synthetic 502", records[0].Err)
	require.Empty(t, records[0].DocumentID)

	// after the fail-next budget is exhausted, the next call succeeds
	got, err := c.SendToSign(context.Background(), trustme.SendToSignInput{
		AdditionalInfo: "ct-1",
		Requisites: []trustme.Requisite{{
			FIO: "X", IINBIN: "1", PhoneNumber: "+77",
		}},
	})
	require.NoError(t, err)
	require.NotEmpty(t, got.DocumentID)
}

func TestSpyClient_SearchContractByAdditionalInfo(t *testing.T) {
	t.Parallel()

	t.Run("not registered returns ErrTrustMeNotFound", func(t *testing.T) {
		t.Parallel()
		c, _ := newClient(t)
		_, err := c.SearchContractByAdditionalInfo(context.Background(), "ct-missing")
		require.True(t, errors.Is(err, trustme.ErrTrustMeNotFound))
	})

	t.Run("registered returns document", func(t *testing.T) {
		t.Parallel()
		c, store := newClient(t)
		store.RegisterDocument("ct-1", "doc-xyz", "https://tct.kz/uploader/doc-xyz", 0)
		got, err := c.SearchContractByAdditionalInfo(context.Background(), "ct-1")
		require.NoError(t, err)
		require.Equal(t, &trustme.SearchContractResult{
			DocumentID:     "doc-xyz",
			ShortURL:       "https://tct.kz/uploader/doc-xyz",
			ContractStatus: 0,
		}, got)
	})

	t.Run("empty additionalInfo errors", func(t *testing.T) {
		t.Parallel()
		c, _ := newClient(t)
		_, err := c.SearchContractByAdditionalInfo(context.Background(), "")
		require.Error(t, err)
	})
}

func TestSpyClient_DownloadContractFile(t *testing.T) {
	t.Parallel()

	c, _ := newClient(t)

	t.Run("empty id errors", func(t *testing.T) {
		t.Parallel()
		_, err := c.DownloadContractFile(context.Background(), "")
		require.Error(t, err)
	})

	t.Run("returns synthetic bytes", func(t *testing.T) {
		t.Parallel()
		body, err := c.DownloadContractFile(context.Background(), "doc-xyz")
		require.NoError(t, err)
		require.Equal(t, []byte("spy-signed-doc-xyz"), body)
	})
}

func TestSpyStore_RingEvictsOldest(t *testing.T) {
	t.Parallel()
	store := NewSpyStore()
	for i := 0; i < storeCapacity+5; i++ {
		store.Record(SentRecord{AdditionalInfo: "x"})
	}
	require.Len(t, store.List(), storeCapacity)
}
