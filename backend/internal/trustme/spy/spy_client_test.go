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
	store.now = func() time.Time { return fixed }
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
		NumberDial:     "UGC-7",
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
	require.Equal(t, "UGC-7", records[0].NumberDial)
	require.Equal(t, "Иванов Иван Иванович", records[0].FIO)
	require.Equal(t, "880101300123", records[0].IIN)
	require.Equal(t, "+77071234567", records[0].Phone)
	require.Equal(t, HashPDFBase64("JVBERi0xLg=="), records[0].PDFSha256)
	require.Empty(t, records[0].Err)
}

func TestHashPDFBase64_Empty(t *testing.T) {
	t.Parallel()
	require.Equal(t, "", HashPDFBase64(""))
	require.Equal(t, HashPDFBase64("X"), HashPDFBase64("X"))
	require.NotEqual(t, HashPDFBase64("A"), HashPDFBase64("B"))
	require.Len(t, HashPDFBase64("X"), 64)
}

func TestSpyClient_SendToSign_FailNext(t *testing.T) {
	t.Parallel()

	c, store := newClient(t)
	store.RegisterFailNext("880101300123", "synthetic 502", 1)

	// SendToSign with a different IIN must NOT consume the registered failure.
	otherIIN, err := c.SendToSign(context.Background(), trustme.SendToSignInput{
		AdditionalInfo: "ct-other",
		Requisites: []trustme.Requisite{{
			FIO: "Y", IINBIN: "999999999999", PhoneNumber: "+77",
		}},
	})
	require.NoError(t, err)
	require.NotEmpty(t, otherIIN.DocumentID)

	// SendToSign with the registered IIN consumes it: error returned, error
	// recorded in spy.
	_, err = c.SendToSign(context.Background(), trustme.SendToSignInput{
		AdditionalInfo: "ct-1",
		Requisites: []trustme.Requisite{{
			FIO: "X", IINBIN: "880101300123", PhoneNumber: "+77",
		}},
	})
	require.Error(t, err)
	require.Equal(t, "synthetic 502", err.Error())

	records := store.List()
	require.Len(t, records, 2)
	require.Empty(t, records[0].Err, "first call (other IIN) succeeded")
	require.NotEmpty(t, records[0].DocumentID)
	require.Equal(t, "synthetic 502", records[1].Err, "second call (matching IIN) failed")
	require.Empty(t, records[1].DocumentID)

	// After the count budget is exhausted, the next call on the same IIN succeeds.
	got, err := c.SendToSign(context.Background(), trustme.SendToSignInput{
		AdditionalInfo: "ct-1",
		Requisites: []trustme.Requisite{{
			FIO: "X", IINBIN: "880101300123", PhoneNumber: "+77",
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
	base := time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC)
	store := NewSpyStore()
	store.now = func() time.Time { return base }
	for i := 0; i < storeCapacity+5; i++ {
		store.Record(SentRecord{AdditionalInfo: "x", SentAt: base})
	}
	require.Len(t, store.List(), storeCapacity)
}

func TestSpyStore_TTLEvictsOldRecords(t *testing.T) {
	t.Parallel()
	base := time.Date(2026, 5, 9, 12, 0, 0, 0, time.UTC)
	store := NewSpyStore()
	store.now = func() time.Time { return base }

	store.Record(SentRecord{AdditionalInfo: "old", SentAt: base})
	store.Record(SentRecord{AdditionalInfo: "fresh", SentAt: base.Add(30 * time.Minute)})

	store.now = func() time.Time { return base.Add(90 * time.Minute) }
	list := store.List()
	require.Len(t, list, 1)
	require.Equal(t, "fresh", list[0].AdditionalInfo)
}
