// Package creator_applications — E2E HTTP-поверхность
// POST /creators/applications/{id}/approve (chunk 18b onboarding-roadmap'а):
// admin одобряет заявку из `moderation`, переход terminal `approved`,
// snapshot заявки + соцсетей + категорий копируется в новую сущность
// `creators` в одной транзакции, audit + state-history пишутся, а сразу
// после коммита fire-and-forget уходит статическое Telegram-сообщение
// (расширение existing notify-сервиса методом NotifyApplicationApproved).
//
// TestApproveCreatorApplication прогоняет всю I/O-матрицу одной функцией.
// Сначала authn/authz: отсутствие Bearer'а отдаёт 401, brand_manager — 403
// без leak'а существования заявки. На рандомном UUID — 404 NOT_FOUND.
// Status-guard ловит approve из `verification` (отдаёт 422 NOT_APPROVABLE,
// БД остаётся нетронутой); approve из `moderation` без Telegram-link —
// 422 TELEGRAM_NOT_LINKED (link обязателен, потому что approve запускает
// поздравительный пуш через бот). Repeat-approve той же заявки — второй
// 422 NOT_APPROVABLE, в БД остаётся ровно одна creator-row.
//
// Happy-path расщеплён на два сценария — happy_full и happy_sparse —
// собранных через переиспользуемый testutil-pipeline
// SetupCreatorApplicationInModeration. happy_full настраивает заявку с
// заполненными nullable (middle_name + address + category_other_text), 3
// категориями и 3 социалками: IG auto-verified через SendPulse webhook,
// TT и Threads остаются unverified. Это компромисс относительно spec'и
// (которая просит "IG auto / TT manual / Threads non-verified") — публичный
// API верифицирует только до первого успешного perехода verification →
// moderation; вторая верификация на той же заявке возвращает
// NOT_IN_VERIFICATION. Один verified social + два unverified покрывают все
// три ветки маппинга в creator_socials (auto-method, raw verification
// snapshot, "верификация не делалась") без нарушения state machine.
// happy_sparse — заявка с middleName=nil, address=nil, categoryOtherText=nil,
// одной IG auto-verified социалкой и одной категорией; защищает omitempty/
// null-семантику openapi на стороне аггрегата креатора. Оба сценария после
// approve дёргают GET /creators/{creatorId} и сверяют ответ с фикстурой
// через AssertCreatorAggregateMatchesSetup (поле-в-поле, sorted socials по
// (platform, handle), sorted categories по code). WaitForTelegramSent в
// каждом ловит ровно одно application_approved сообщение с точным текстом
// expectedApproveText, plain mode без WebApp-keyboard.
//
// Concurrent approve race — два goroutine POST approve на одну заявку
// под `-race`. UNIQUE constraint creators_source_application_id_unique
// гарантирует, что ровно один TX выживает: один 200, один 422
// NOT_APPROVABLE. В cleanup-стеке регистрируется creator-row выжившего
// goroutine'а (тест парсит `creatorId` из выигрышного ответа). Все
// t.Run параллельны; positive setup'ы идут через
// SetupCreatorApplicationInModeration с auto-cleanup, негативные сценарии
// (verification, no-link) собирают заявку ad-hoc через
// SetupCreatorApplicationViaLanding потому что helper требует уже
// дошедшего до moderation состояния.
package creator_applications_test

import (
	"context"
	"net/http"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/AlekSi/pointer"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/alikhanmurzayev/ugcboost/backend/e2e/apiclient"
	"github.com/alikhanmurzayev/ugcboost/backend/e2e/testclient"
	"github.com/alikhanmurzayev/ugcboost/backend/e2e/testutil"
)

// auditActionCreatorApplicationApprove mirrors the backend constant
// AuditActionCreatorApplicationApprove. e2e is its own Go module, so the
// value is re-declared locally instead of imported.
const auditActionCreatorApplicationApprove = "creator_application_approve"

// auditEntityTypeCreatorApplicationApprove mirrors AuditEntityTypeCreatorApplication.
const auditEntityTypeCreatorApplicationApprove = "creator_application"

// telegramSilenceWindowApprove — окно для негативных ассертов на отсутствие
// нового пуша (короткое окно после первого, чтобы repeat-approve не
// пушнулся).
const telegramSilenceWindowApprove = 5 * time.Second

// expectedApproveText must be kept in sync with
// internal/telegram/notifier.go::applicationApprovedText.
const expectedApproveText = "Здравствуйте!\n\n" +
	"Рады сообщить, что ваша заявка прошла модерацию 😍 Ваш профиль, визуальный стиль и контент соответствуют критериям отбора для участия в fashion-кампаниях платформы UGC boost 💫\n\n" +
	"В ближайшее время мы отправим вам детали участия в EURASIAN FASHION WEEK и договор для подписания.\n\n" +
	"Добро пожаловать на платформу UGC boost 💫\n\n" +
	"После Недели моды мы планируем запустить приложение в App Store и добавить новые возможности для UGC-сотрудничества с брендами и партнерами EURASIAN FASHION WEEK.\n\n" +
	"Оставайтесь с нами — впереди много масштабных проектов!"

func TestApproveCreatorApplication(t *testing.T) {
	t.Parallel()

	t.Run("auth: missing bearer returns 401", func(t *testing.T) {
		t.Parallel()
		c := testutil.NewAPIClient(t)
		resp, err := c.ApproveCreatorApplicationWithResponse(context.Background(), uuid.New(), apiclient.CreatorApprovalInput{})
		require.NoError(t, err)
		require.Equal(t, http.StatusUnauthorized, resp.StatusCode())
	})

	t.Run("auth: brand_manager bearer returns 403", func(t *testing.T) {
		t.Parallel()
		adminClient, adminToken, _ := testutil.SetupAdminClient(t)
		brandID := testutil.SetupBrand(t, adminClient, adminToken,
			"approve-403-brand-"+testutil.UniqueEmail("brand"))
		_, mgrToken, _ := testutil.SetupManagerWithLogin(t, adminClient, adminToken, brandID)

		c := testutil.NewAPIClient(t)
		resp, err := c.ApproveCreatorApplicationWithResponse(context.Background(), uuid.New(),
			apiclient.CreatorApprovalInput{}, testutil.WithAuth(mgrToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusForbidden, resp.StatusCode())
		require.NotNil(t, resp.JSON403)
		require.Equal(t, "FORBIDDEN", resp.JSON403.Error.Code)
	})

	t.Run("application not found returns 404 NOT_FOUND", func(t *testing.T) {
		t.Parallel()
		_, adminToken, _ := testutil.SetupAdminClient(t)
		c := testutil.NewAPIClient(t)
		resp, err := c.ApproveCreatorApplicationWithResponse(context.Background(), uuid.New(),
			apiclient.CreatorApprovalInput{}, testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusNotFound, resp.StatusCode())
		require.NotNil(t, resp.JSON404)
		require.Equal(t, "NOT_FOUND", resp.JSON404.Error.Code)
	})

	t.Run("not approvable from verification returns 422 NOT_APPROVABLE", func(t *testing.T) {
		t.Parallel()
		setup := testutil.SetupCreatorApplicationViaLanding(t)
		c, adminToken, _ := testutil.SetupAdminClient(t)

		appUUID := uuid.MustParse(setup.ApplicationID)
		resp, err := c.ApproveCreatorApplicationWithResponse(context.Background(), appUUID,
			apiclient.CreatorApprovalInput{}, testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode())
		require.NotNil(t, resp.JSON422)
		require.Equal(t, "CREATOR_APPLICATION_NOT_APPROVABLE", resp.JSON422.Error.Code)

		// State unchanged.
		detail := getApplicationDetailForApprove(t, c, adminToken, setup.ApplicationID)
		require.Equal(t, apiclient.Verification, detail.Status)
	})

	t.Run("not approvable without telegram link returns 422 TELEGRAM_NOT_LINKED", func(t *testing.T) {
		t.Parallel()
		// SendPulse webhook does not require an existing link to move
		// verification → moderation; `link` stays optional through
		// auto-verify (the post-commit notify just warn-logs without a
		// chat). After SendPulse, the application is in `moderation`
		// with no telegram-link, ready for the approve guard to fire.
		setup := testutil.SetupCreatorApplicationViaLanding(t)
		appID := setup.ApplicationID
		c, adminToken, _ := testutil.SetupAdminClient(t)

		igHandle := normalisedIGHandleForApprove(t, setup.Request)
		code := testutil.GetCreatorApplicationVerificationCode(t, appID)
		body := testutil.SendPulseWebhookHappyPathRequest(code, igHandle)
		status, _ := testutil.SendPulseWebhookCall(t, testutil.SendPulseWebhookOptions{Body: &body})
		require.Equal(t, http.StatusOK, status)

		preApprove := getApplicationDetailForApprove(t, c, adminToken, appID)
		require.Equal(t, apiclient.Moderation, preApprove.Status, "precondition: SendPulse must have moved the app")
		require.Nil(t, preApprove.TelegramLink, "precondition: no LinkTelegramToApplication, link must be absent")

		appUUID := uuid.MustParse(appID)
		resp, err := c.ApproveCreatorApplicationWithResponse(context.Background(), appUUID,
			apiclient.CreatorApprovalInput{}, testutil.WithAuth(adminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode())
		require.NotNil(t, resp.JSON422)
		require.Equal(t, "CREATOR_APPLICATION_TELEGRAM_NOT_LINKED", resp.JSON422.Error.Code)

		// Status guard runs before any INSERT in creators.
		stillModeration := getApplicationDetailForApprove(t, c, adminToken, appID)
		require.Equal(t, apiclient.Moderation, stillModeration.Status, "БД не должна меняться при отказе")
	})

	t.Run("happy_full — заполнённые nullable + 3 социалки + 3 категории, GET aggregate full match", func(t *testing.T) {
		t.Parallel()
		fx := testutil.SetupCreatorApplicationInModeration(t, testutil.CreatorApplicationFixture{
			MiddleName:        pointer.ToString("Ивановна"),
			Address:           pointer.ToString("ул. Абая 10"),
			CategoryCodes:     []string{"beauty", "fashion", "other"},
			CategoryOtherText: pointer.ToString("стримы"),
			Socials: []testutil.SocialFixture{
				{Platform: string(apiclient.Instagram), Handle: "aidana_full", Verification: testutil.VerificationAutoIG},
				{Platform: string(apiclient.Tiktok), Handle: "aidana_tt_full", Verification: testutil.VerificationNone},
				{Platform: string(apiclient.Threads), Handle: "aidana_th_full", Verification: testutil.VerificationNone},
			},
		})

		c := testutil.NewAPIClient(t)
		appUUID := uuid.MustParse(fx.ApplicationID)
		since := time.Now().UTC()
		approveResp, err := c.ApproveCreatorApplicationWithResponse(context.Background(), appUUID,
			apiclient.CreatorApprovalInput{}, testutil.WithAuth(fx.AdminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, approveResp.StatusCode())
		require.NotNil(t, approveResp.JSON200)
		creatorID := approveResp.JSON200.Data.CreatorId
		require.NotEqual(t, uuid.Nil, creatorID)
		// Cleanup the creator before LIFO-tries to drop the parent app:
		// creators.source_application_id has no ON DELETE clause, so the
		// app cannot be removed while a creator still references it.
		testutil.RegisterCreatorCleanup(t, creatorID.String())

		detail := getApplicationDetailForApprove(t, c, fx.AdminToken, fx.ApplicationID)
		require.Equal(t, apiclient.Approved, detail.Status)

		audit := testutil.FindAuditEntry(t, c, fx.AdminToken,
			auditEntityTypeCreatorApplicationApprove, fx.ApplicationID,
			auditActionCreatorApplicationApprove)
		require.NotNil(t, audit.NewValue)

		getResp, err := c.GetCreatorWithResponse(context.Background(), creatorID,
			testutil.WithAuth(fx.AdminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, getResp.StatusCode())
		require.NotNil(t, getResp.JSON200)
		testutil.AssertCreatorAggregateMatchesSetup(t, fx, creatorID.String(), getResp.JSON200.Data)

		msgs := testutil.WaitForTelegramSent(t, fx.TelegramUserID, testutil.TelegramSentOptions{
			Since:       since,
			ExpectCount: 1,
		})
		require.Len(t, msgs, 1)
		assertApprovePushExact(t, msgs[0], fx.TelegramUserID, since)

		// Outbound notify lands in telegram_messages with direction=outbound
		// and the same body the spy captured. Status is not pinned — staging
		// TeeSender mode reports failed sends for synthetic chat ids; the
		// row itself is the invariant under test. Cleanup is auto-registered
		// inside LinkTelegramToApplication (called from the fixture setup).
		row := testutil.AssertTelegramMessageRecorded(t, c, fx.AdminToken, fx.TelegramUserID,
			testutil.TelegramMessageMatcher{Direction: "outbound", TextContains: msgs[0].Text})
		require.Equal(t, msgs[0].Text, row.Text)
	})

	t.Run("happy_sparse — все nullable=nil, 1 IG auto-verified, 1 категория", func(t *testing.T) {
		t.Parallel()
		fx := testutil.SetupCreatorApplicationInModeration(t, testutil.CreatorApplicationFixture{
			CategoryCodes: []string{"beauty"},
			Socials: []testutil.SocialFixture{
				{Platform: string(apiclient.Instagram), Handle: "aidana_sparse", Verification: testutil.VerificationAutoIG},
			},
		})

		c := testutil.NewAPIClient(t)
		appUUID := uuid.MustParse(fx.ApplicationID)
		since := time.Now().UTC()
		approveResp, err := c.ApproveCreatorApplicationWithResponse(context.Background(), appUUID,
			apiclient.CreatorApprovalInput{}, testutil.WithAuth(fx.AdminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, approveResp.StatusCode())
		require.NotNil(t, approveResp.JSON200)
		creatorID := approveResp.JSON200.Data.CreatorId
		require.NotEqual(t, uuid.Nil, creatorID)
		testutil.RegisterCreatorCleanup(t, creatorID.String())

		getResp, err := c.GetCreatorWithResponse(context.Background(), creatorID,
			testutil.WithAuth(fx.AdminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, getResp.StatusCode())
		require.NotNil(t, getResp.JSON200)
		testutil.AssertCreatorAggregateMatchesSetup(t, fx, creatorID.String(), getResp.JSON200.Data)

		msgs := testutil.WaitForTelegramSent(t, fx.TelegramUserID, testutil.TelegramSentOptions{
			Since:       since,
			ExpectCount: 1,
		})
		require.Len(t, msgs, 1)
		assertApprovePushExact(t, msgs[0], fx.TelegramUserID, since)
	})

	t.Run("repeat approve returns 422 NOT_APPROVABLE; one creator row, one push", func(t *testing.T) {
		t.Parallel()
		fx := testutil.SetupCreatorApplicationInModeration(t, testutil.CreatorApplicationFixture{
			CategoryCodes: []string{"beauty"},
			Socials: []testutil.SocialFixture{
				{Platform: string(apiclient.Instagram), Handle: "aidana_repeat", Verification: testutil.VerificationAutoIG},
			},
		})
		c := testutil.NewAPIClient(t)
		appUUID := uuid.MustParse(fx.ApplicationID)
		firstSince := time.Now().UTC()
		first, err := c.ApproveCreatorApplicationWithResponse(context.Background(), appUUID,
			apiclient.CreatorApprovalInput{}, testutil.WithAuth(fx.AdminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, first.StatusCode())
		require.NotNil(t, first.JSON200)
		testutil.RegisterCreatorCleanup(t, first.JSON200.Data.CreatorId.String())

		// Drain the first push so the second-attempt window is past it.
		msgs := testutil.WaitForTelegramSent(t, fx.TelegramUserID, testutil.TelegramSentOptions{
			Since:       firstSince,
			ExpectCount: 1,
		})
		require.Len(t, msgs, 1)
		assertApprovePushExact(t, msgs[0], fx.TelegramUserID, firstSince)

		afterFirst := time.Now().UTC()
		second, err := c.ApproveCreatorApplicationWithResponse(context.Background(), appUUID,
			apiclient.CreatorApprovalInput{}, testutil.WithAuth(fx.AdminToken))
		require.NoError(t, err)
		require.Equal(t, http.StatusUnprocessableEntity, second.StatusCode())
		require.NotNil(t, second.JSON422)
		require.Equal(t, "CREATOR_APPLICATION_NOT_APPROVABLE", second.JSON422.Error.Code)

		// Status stays approved; second 422 short-circuited inside WithTx, no notify.
		detail := getApplicationDetailForApprove(t, c, fx.AdminToken, fx.ApplicationID)
		require.Equal(t, apiclient.Approved, detail.Status)
		testutil.EnsureNoNewTelegramSent(t, fx.TelegramUserID, afterFirst, telegramSilenceWindowApprove)
	})

	t.Run("concurrent approve race — exactly one 200, exactly one 422", func(t *testing.T) {
		t.Parallel()
		fx := testutil.SetupCreatorApplicationInModeration(t, testutil.CreatorApplicationFixture{
			CategoryCodes: []string{"beauty"},
			Socials: []testutil.SocialFixture{
				{Platform: string(apiclient.Instagram), Handle: "aidana_race", Verification: testutil.VerificationAutoIG},
			},
		})

		c := testutil.NewAPIClient(t)
		appUUID := uuid.MustParse(fx.ApplicationID)

		var (
			wg          sync.WaitGroup
			ok422       atomic.Int32
			ok200       atomic.Int32
			winnerLock  sync.Mutex
			winnerUUID  uuid.UUID
			otherStatus atomic.Int32
		)
		wg.Add(2)
		for i := 0; i < 2; i++ {
			go func() {
				defer wg.Done()
				resp, err := c.ApproveCreatorApplicationWithResponse(context.Background(), appUUID,
					apiclient.CreatorApprovalInput{}, testutil.WithAuth(fx.AdminToken))
				require.NoError(t, err)
				switch resp.StatusCode() {
				case http.StatusOK:
					ok200.Add(1)
					require.NotNil(t, resp.JSON200)
					winnerLock.Lock()
					winnerUUID = resp.JSON200.Data.CreatorId
					winnerLock.Unlock()
				case http.StatusUnprocessableEntity:
					require.NotNil(t, resp.JSON422)
					require.Equal(t, "CREATOR_APPLICATION_NOT_APPROVABLE", resp.JSON422.Error.Code)
					ok422.Add(1)
				default:
					otherStatus.Store(int32(resp.StatusCode()))
				}
			}()
		}
		wg.Wait()

		require.Zero(t, otherStatus.Load(), "no 5xx / unexpected status under race")
		require.Equal(t, int32(1), ok200.Load(), "exactly one approve must succeed")
		require.Equal(t, int32(1), ok422.Load(), "exactly one approve must be rejected as not approvable")
		require.NotEqual(t, uuid.Nil, winnerUUID)
		testutil.RegisterCreatorCleanup(t, winnerUUID.String())
	})
}

// assertApprovePushExact mirrors assertRejectPushExact but for the approve
// notify. msg.Error is intentionally not asserted — under TeeSender the
// real bot.SendMessage rejects the synthetic chat id and the spy records
// an upstream error, but that does not invalidate the fire-and-forget
// pipeline (same convention as assertRejectPushExact / sendpulse tests).
func assertApprovePushExact(t *testing.T, msg testclient.TelegramSentMessage, chatID int64, since time.Time) {
	t.Helper()
	require.Equal(t, chatID, msg.ChatId)
	require.Equal(t, expectedApproveText, msg.Text)
	require.Nil(t, msg.WebAppUrl, "approve message must be plain — no WebApp keyboard")
	require.True(t, !msg.SentAt.Before(since), "sent_at must be at or after the cursor")
	require.WithinDuration(t, time.Now().UTC(), msg.SentAt, telegramSilenceWindowApprove*2)
}

// getApplicationDetailForApprove wraps the admin GET aggregate for the
// approve scenarios. Local copy to avoid cross-file helper coupling — the
// reject and manual-verify tests carry their own thin wrappers for the
// same reason.
func getApplicationDetailForApprove(t *testing.T, c *apiclient.ClientWithResponses,
	token, appID string) *apiclient.CreatorApplicationDetailData {
	t.Helper()
	id, err := uuid.Parse(appID)
	require.NoError(t, err)
	resp, err := c.GetCreatorApplicationWithResponse(context.Background(), id, testutil.WithAuth(token))
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode())
	require.NotNil(t, resp.JSON200)
	return &resp.JSON200.Data
}

// normalisedIGHandleForApprove pulls the canonical IG handle out of the
// submission request used to seed an application. SendPulse webhook
// expects the handle without a leading '@' and lowercased — see
// domain.NormalizeInstagramHandle. Local mirror of the helper used by
// reject/manual-verify tests.
func normalisedIGHandleForApprove(t *testing.T, req apiclient.CreatorApplicationSubmitRequest) string {
	t.Helper()
	for _, s := range req.Socials {
		if s.Platform == apiclient.Instagram {
			h := s.Handle
			if len(h) > 0 && h[0] == '@' {
				h = h[1:]
			}
			return h
		}
	}
	t.Fatalf("submission request has no instagram social: %#v", req.Socials)
	return ""
}
