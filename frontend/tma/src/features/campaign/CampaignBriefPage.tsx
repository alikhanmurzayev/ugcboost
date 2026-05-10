import { useEffect, useState } from "react";
import { useParams } from "react-router-dom";
import { getCampaignByToken } from "./campaigns";
import { AcceptedView } from "./AcceptedView";
import { ConfirmDialog } from "./ConfirmDialog";
import { DeclinedView } from "./DeclinedView";
import { NdaGate } from "./NdaGate";
import { NotFoundPage } from "./NotFoundPage";
import type {
  CampaignBrief,
  CooperationSection,
  Designer,
  DesignersSection,
  Mentions,
  PartnerSection,
  ReelsBrief,
  StoriesBrief,
} from "./types";
import { useAgreeDecision, useDeclineDecision } from "./useDecision";
import { decisionErrorMessage } from "../../shared/i18n/errors";
import { CAMPAIGN_CREATOR_STATUS } from "../../shared/constants/campaignCreatorStatus";

type ConfirmTarget = "accept" | "decline" | null;

export function CampaignBriefPage() {
  const { token } = useParams<{ token: string }>();
  const campaign = token ? getCampaignByToken(token) : undefined;
  const [ndaAccepted, setNdaAccepted] = useState(false);
  const [confirm, setConfirm] = useState<ConfirmTarget>(null);
  const agree = useAgreeDecision(token);
  const decline = useDeclineDecision(token);

  useEffect(() => {
    const lock = !ndaAccepted || confirm !== null;
    if (lock) {
      const prev = document.body.style.overflow;
      document.body.style.overflow = "hidden";
      return () => {
        document.body.style.overflow = prev;
      };
    }
  }, [ndaAccepted, confirm]);

  if (!campaign) {
    return <NotFoundPage />;
  }

  const decisionResult = agree.data ?? decline.data;
  if (decisionResult?.status === CAMPAIGN_CREATOR_STATUS.AGREED) {
    return <AcceptedView alreadyDecided={decisionResult.alreadyDecided} />;
  }
  if (decisionResult?.status === CAMPAIGN_CREATOR_STATUS.DECLINED) {
    return <DeclinedView alreadyDecided={decisionResult.alreadyDecided} />;
  }

  const handleAcceptClick = () => setConfirm("accept");
  const handleDeclineClick = () => setConfirm("decline");

  const handleConfirmAccept = () => {
    setConfirm(null);
    agree.mutate();
  };

  const handleConfirmDecline = () => {
    setConfirm(null);
    decline.mutate();
  };

  const submitting = agree.isPending || decline.isPending;
  const submitError = agree.error ?? decline.error;

  const handleCancel = () => setConfirm(null);

  return (
    <div className="min-h-screen bg-surface pb-44">
      <div
        aria-hidden={!ndaAccepted}
        className={
          ndaAccepted ? "" : "pointer-events-none select-none blur-md"
        }
      >
      <Header campaign={campaign} />

      <main className="mx-auto max-w-xl space-y-6 px-4 py-6">
        {campaign.eventDetails && (
          <Card>
            <EventDetailsList details={campaign.eventDetails} />
            {campaign.cooperationFormat && (
              <div className="mt-4 border-t border-surface-200 pt-4">
                <DetailRow
                  label="Формат сотрудничества"
                  value={campaign.cooperationFormat}
                />
              </div>
            )}
          </Card>
        )}

        {(campaign.fromBrand || campaign.fromCreator) && (
          <Section title="Условия коллаборации">
            <div className="grid gap-4">
              {campaign.fromBrand && (
                <CooperationCard section={campaign.fromBrand} accent />
              )}
              {campaign.fromCreator && (
                <CooperationCard section={campaign.fromCreator} />
              )}
            </div>
          </Section>
        )}

        {(campaign.reels || campaign.stories) && (
          <Section title="ТЗ по контенту">
            <ContentCard
              reels={campaign.reels}
              stories={campaign.stories}
            />
          </Section>
        )}

        {campaign.mentions && (
          <Section title={campaign.mentions.title ?? "Отметки"}>
            <MentionsCard mentions={campaign.mentions} />
          </Section>
        )}

        {campaign.partner && (
          <Section title={campaign.partner.title}>
            <PartnerCard partner={campaign.partner} />
          </Section>
        )}

        {campaign.designers && campaign.designers.items.length > 0 && (
          <Section title={campaign.designers.title ?? "Дизайнеры"}>
            <DesignersCard designers={campaign.designers} />
          </Section>
        )}

        {campaign.aboutParagraphs && campaign.aboutParagraphs.length > 0 && (
          <Section title={campaign.aboutTitle ?? "О коллекции"}>
            {campaign.aboutNote && (
              <p className="mb-3 px-1 text-xs italic leading-relaxed text-gray-500">
                {campaign.aboutNote}
              </p>
            )}
            <Card>
              {campaign.aboutImageUrl && (
                <img
                  src={campaign.aboutImageUrl}
                  alt={
                    campaign.aboutImageAlt ??
                    campaign.aboutTitle ??
                    "О коллекции"
                  }
                  className="mb-4 w-full rounded-card"
                />
              )}
              <div className="space-y-4 text-sm leading-relaxed text-gray-700">
                {campaign.aboutParagraphs.map((p, i) => (
                  <p key={i} className="whitespace-pre-line">
                    {renderBoldMarkdown(p)}
                  </p>
                ))}
              </div>
            </Card>
          </Section>
        )}

      </main>
      </div>
      <div
        aria-hidden={!ndaAccepted}
        className={
          "pointer-events-none fixed inset-x-0 bottom-0 z-40 " +
          (ndaAccepted ? "" : "select-none blur-md")
        }
      >
        <div className="h-8 bg-gradient-to-b from-transparent to-surface" />
        <div className="bg-surface pb-[calc(env(safe-area-inset-bottom)+1rem)]">
          <div className="pointer-events-auto mx-auto max-w-xl space-y-3 px-4">
            {submitError && (
              <p
                data-testid="tma-decision-error"
                data-error-code={submitError.code}
                className="rounded-md bg-red-50 px-4 py-2 text-center text-sm text-red-700"
              >
                {decisionErrorMessage(submitError.code)}
              </p>
            )}
            <button
              type="button"
              onClick={handleAcceptClick}
              disabled={submitting}
              className="w-full rounded-full bg-primary py-4 text-base font-semibold text-white shadow-xl shadow-primary/30 transition-all hover:bg-primary-600 hover:shadow-2xl active:bg-primary-700 active:shadow-md disabled:opacity-60"
              data-testid="campaign-accept-button"
            >
              Согласиться
            </button>
            <button
              type="button"
              onClick={handleDeclineClick}
              disabled={submitting}
              className="block w-full text-center text-sm font-medium text-gray-400 underline-offset-2 transition-colors hover:text-gray-600 hover:underline disabled:opacity-60"
              data-testid="campaign-decline-button"
            >
              Отказаться
            </button>
          </div>
        </div>
      </div>
      {!ndaAccepted && <NdaGate onAccept={() => setNdaAccepted(true)} />}
      {ndaAccepted && confirm === "accept" && (
        <ConfirmDialog
          title="Согласиться на коллаборацию?"
          description={
            <ol className="ml-5 list-decimal space-y-2">
              <li>Подтверждая, вы принимаете условия ТЗ</li>
              <li>
                После этого мы отправим вам договор о сотрудничестве на
                указанный номер телефона ссылкой по СМС
              </li>
              <li>
                Договор нужно подписать онлайн через СМС (инструкция будет
                по ссылке в СМС)
              </li>
              <li>
                После подписания договора ожидайте онлайн-пригласительный
                билет на показы {campaign.inviteEventLabel}
              </li>
            </ol>
          }
          confirmText="Да, согласиться"
          cancelText="Отмена"
          confirmVariant="primary"
          onConfirm={handleConfirmAccept}
          onCancel={handleCancel}
          testIdPrefix="accept"
        />
      )}
      {ndaAccepted && confirm === "decline" && (
        <ConfirmDialog
          title="Отказаться от коллаборации?"
          description="Подтверждая, вы отказываетесь от участия в этой коллаборации."
          confirmText="Да, отказаться"
          cancelText="Отмена"
          confirmVariant="secondary"
          onConfirm={handleConfirmDecline}
          onCancel={handleCancel}
          testIdPrefix="decline"
        />
      )}
    </div>
  );
}

function Header({ campaign }: { campaign: CampaignBrief }) {
  return (
    <header className="bg-gradient-to-br from-gray-900 to-gray-700 px-4 pb-8 pt-6 text-white">
      <div className="mx-auto max-w-xl">
        <EfwLogo />
        <p className="mt-8 text-xs font-semibold uppercase tracking-wider text-primary-300">
          {campaign.brandName}
        </p>
        <h1 className="mt-2 whitespace-pre-line text-3xl font-bold leading-tight">
          {campaign.campaignTitle}
        </h1>
        {campaign.subtitle && (
          <p
            className={
              campaign.subtitleAsTagline
                ? "mt-1 text-xs text-gray-300"
                : "mt-2 text-base text-gray-200"
            }
          >
            {campaign.subtitle}
          </p>
        )}
        {campaign.context && (
          <p className="mt-3 text-sm leading-relaxed text-gray-300">
            {campaign.context}
          </p>
        )}
      </div>
    </header>
  );
}

function EfwLogo() {
  return (
    <div className="flex justify-center">
      <img
        src="/logos/efw-white.png"
        alt="Eurasian Fashion Week"
        className="h-16 w-auto"
      />
    </div>
  );
}

function Section({
  title,
  children,
}: {
  title: string;
  children: React.ReactNode;
}) {
  return (
    <section>
      <h2 className="mb-3 px-1 text-lg font-bold text-gray-900">{title}</h2>
      {children}
    </section>
  );
}

function Card({
  children,
  accent,
}: {
  children: React.ReactNode;
  accent?: boolean;
}) {
  return (
    <div
      className={
        "rounded-card p-5 " +
        (accent
          ? "border border-primary-200 bg-primary-50"
          : "border border-surface-200 bg-white")
      }
    >
      {children}
    </div>
  );
}

function EventDetailsList({ details }: { details: { label: string; value: string }[] }) {
  return (
    <dl className="space-y-3">
      {details.map((d, i) => (
        <DetailRow key={i} label={d.label} value={d.value} />
      ))}
    </dl>
  );
}

function DetailRow({
  label,
  value,
  emphasized,
}: {
  label: string;
  value: string;
  emphasized?: boolean;
}) {
  return (
    <div className="flex flex-col gap-0.5">
      <dt className="text-xs font-semibold uppercase tracking-wide text-gray-500">
        {label}
      </dt>
      <dd
        className={
          "text-sm leading-snug text-gray-900 " +
          (emphasized ? "font-bold" : "")
        }
      >
        {value}
      </dd>
    </div>
  );
}

function CooperationCard({
  section,
  accent,
}: {
  section: CooperationSection;
  accent?: boolean;
}) {
  return (
    <Card accent={accent}>
      <h3 className="text-sm font-bold uppercase tracking-wide text-gray-900">
        {section.title}
      </h3>
      <ul className="mt-3 space-y-2">
        {section.items.map((item, i) => (
          <BulletItem key={i}>{item}</BulletItem>
        ))}
      </ul>
    </Card>
  );
}

function ContentCard({
  reels,
  stories,
}: {
  reels?: ReelsBrief;
  stories?: StoriesBrief;
}) {
  const hasBoth = !!reels && !!stories;
  return (
    <Card>
      {reels && (
        <div>
          {hasBoth && (
            <h3 className="mb-4 text-sm font-bold uppercase tracking-wide text-gray-900">
              Reels
            </h3>
          )}
          <ReelsContent reels={reels} />
        </div>
      )}
      {stories && (
        <div
          className={
            reels ? "mt-6 border-t border-surface-200 pt-6" : ""
          }
        >
          {hasBoth && (
            <h3 className="mb-4 text-sm font-bold uppercase tracking-wide text-gray-900">
              Stories
            </h3>
          )}
          <StoriesContent stories={stories} />
        </div>
      )}
    </Card>
  );
}

function ReelsContent({ reels }: { reels: ReelsBrief }) {
  return (
    <>
      <DetailRow label="Формат" value={reels.format} />
      <div className="mt-4">
        <DetailRow label="Срок сдачи" value={reels.deadline} emphasized />
      </div>
      {(reels.briefText ||
        (reels.requirements && reels.requirements.length > 0)) && (
        <div className="mt-5 border-t border-surface-200 pt-4">
          <h3 className="text-xs font-semibold uppercase tracking-wide text-gray-500">
            Что снять
          </h3>
          {reels.briefText ? (
            <p className="mt-3 whitespace-pre-line text-sm leading-relaxed text-gray-700">
              {reels.briefText}
            </p>
          ) : reels.requirements ? (
            <ul className="mt-3 space-y-2">
              {reels.requirements.map((item, i) => (
                <BulletItem key={i}>{item}</BulletItem>
              ))}
            </ul>
          ) : null}
        </div>
      )}
      {reels.references && reels.references.length > 0 && (
        <div className="mt-5 border-t border-surface-200 pt-4">
          <h3 className="text-xs font-semibold uppercase tracking-wide text-gray-500">
            Референсы
          </h3>
          <ul className="mt-3 space-y-2">
            {reels.references.map((item, i) => (
              <BulletItem key={i}>{item}</BulletItem>
            ))}
          </ul>
        </div>
      )}
    </>
  );
}

function StoriesContent({ stories }: { stories: StoriesBrief }) {
  return (
    <>
      {stories.count && (
        <DetailRow label="Количество" value={stories.count} />
      )}
      {(stories.briefText ||
        (stories.requirements && stories.requirements.length > 0)) && (
        <div className={stories.count ? "mt-5 border-t border-surface-200 pt-4" : ""}>
          {stories.count && (
            <h3 className="text-xs font-semibold uppercase tracking-wide text-gray-500">
              Что снять
            </h3>
          )}
          {stories.briefText ? (
            <p className={`${stories.count ? "mt-3 " : ""}whitespace-pre-line text-sm leading-relaxed text-gray-700`}>
              {stories.briefText}
            </p>
          ) : stories.requirements ? (
            <ul className={`${stories.count ? "mt-3 " : ""}space-y-2`}>
              {stories.requirements.map((item, i) => (
                <BulletItem key={i}>{item}</BulletItem>
              ))}
            </ul>
          ) : null}
        </div>
      )}
    </>
  );
}

function PartnerCard({ partner }: { partner: PartnerSection }) {
  return (
    <Card>
      <div className="flex gap-4">
        <div className="flex-1 space-y-3 text-sm leading-relaxed text-gray-700">
          {partner.paragraphs.map((p, i) => (
            <p key={i} className="whitespace-pre-line">
              {renderBoldMarkdown(p)}
            </p>
          ))}
        </div>
        {partner.imageUrl && (
          <img
            src={partner.imageUrl}
            alt={partner.imageAlt ?? partner.title}
            className="w-36 flex-shrink-0 self-stretch rounded-card object-cover object-center sm:w-44"
          />
        )}
      </div>
      {partner.handle && (
        <a
          href={instagramUrl(partner.handle)}
          target="_blank"
          rel="noopener noreferrer"
          className="mt-4 inline-flex items-center rounded-full bg-primary-50 px-3 py-1 text-sm font-medium text-primary-700 transition-colors hover:bg-primary-100 active:bg-primary-200"
        >
          {partner.handle}
        </a>
      )}
    </Card>
  );
}

function DesignersCard({ designers }: { designers: DesignersSection }) {
  return (
    <>
      {designers.intro && (
        <p className="mb-3 px-1 text-xs italic leading-relaxed text-gray-500">
          {designers.intro}
        </p>
      )}
      <Card>
        <ul className="space-y-4">
          {designers.items.map((d, i) => (
            <DesignerRow key={i} designer={d} />
          ))}
        </ul>
      </Card>
    </>
  );
}

function DesignerRow({ designer }: { designer: Designer }) {
  const hasHandles = designer.handles && designer.handles.length > 0;
  return (
    <li className="flex flex-col gap-1 text-sm leading-snug">
      <span className="font-semibold text-gray-900">{designer.brand}</span>
      {(designer.designer || hasHandles) && (
        <div className="flex flex-wrap items-center gap-x-2 gap-y-1 text-gray-700">
          {designer.designer && <span>{designer.designer}</span>}
          {designer.handles?.map((h, i) => (
            <span key={h} className="flex items-center gap-x-2">
              {i > 0 && <span aria-hidden="true" className="text-gray-400">×</span>}
              <a
                href={instagramUrl(h)}
                target="_blank"
                rel="noopener noreferrer"
                className="font-medium text-primary-700 hover:text-primary-900"
              >
                {h}
              </a>
            </span>
          ))}
        </div>
      )}
    </li>
  );
}

function MentionsCard({ mentions }: { mentions: Mentions }) {
  return (
    <Card>
      <div className="flex flex-wrap gap-2">
        {mentions.accounts.map((a) => (
          <a
            key={a}
            href={instagramUrl(a)}
            target="_blank"
            rel="noopener noreferrer"
            className="inline-flex items-center rounded-full bg-primary-50 px-3 py-1 text-sm font-medium text-primary-700 transition-colors hover:bg-primary-100 active:bg-primary-200"
          >
            {a}
          </a>
        ))}
      </div>
      {mentions.notes && mentions.notes.length > 0 && (
        <ul className="mt-4 space-y-2 border-t border-surface-200 pt-4">
          {mentions.notes.map((n, i) => (
            <BulletItem key={i}>{n}</BulletItem>
          ))}
        </ul>
      )}
    </Card>
  );
}

function BulletItem({ children }: { children: React.ReactNode }) {
  const content =
    typeof children === "string" ? renderBoldMarkdown(children) : children;
  return (
    <li className="flex items-start gap-2 text-sm leading-relaxed text-gray-700">
      <span
        className="mt-2 inline-block h-1.5 w-1.5 flex-shrink-0 rounded-full bg-primary"
        aria-hidden="true"
      />
      <span className="whitespace-pre-line">{content}</span>
    </li>
  );
}

function instagramUrl(handle: string): string {
  const clean = handle.replace(/^@/, "");
  return `https://instagram.com/${encodeURIComponent(clean)}/`;
}

function renderBoldMarkdown(text: string): React.ReactNode[] {
  const parts = text.split(/(\*\*[^*]+\*\*)/);
  return parts.map((part, i) => {
    if (part.startsWith("**") && part.endsWith("**")) {
      return (
        <strong key={i} className="font-semibold text-gray-900">
          {part.slice(2, -2)}
        </strong>
      );
    }
    return <span key={i}>{part}</span>;
  });
}
