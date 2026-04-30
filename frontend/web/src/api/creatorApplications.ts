import { MOCK_APPLICATIONS } from "@/features/creatorApplications/_mock/applications";
import type {
  Application,
  ApplicationStage,
  QueueCounts,
} from "@/features/creatorApplications/types";

// Mock API for the admin moderation UI. The backend endpoints for these
// stages do not exist yet — we simulate latency so React Query and the
// loading UI behave the same way they will once the real API lands.
//
// Mutations replace the entry in MOCK_APPLICATIONS with a fresh object
// instead of mutating in place. React Query uses structural sharing on
// query results — if a mutation only flips fields on an existing reference,
// the consumer keeps seeing the old object and useMemo dependencies don't
// re-fire. Returning new objects keeps reference equality honest.
const MOCK_LATENCY_MS = 200;

function delay<T>(value: T): Promise<T> {
  return new Promise((resolve) => setTimeout(() => resolve(value), MOCK_LATENCY_MS));
}

function patchApplication(id: string, patch: Partial<Application>): Application {
  const idx = MOCK_APPLICATIONS.findIndex((a) => a.id === id);
  if (idx < 0) throw new Error("Application not found");
  const current = MOCK_APPLICATIONS[idx];
  if (!current) throw new Error("Application not found");
  const next: Application = { ...current, ...patch };
  MOCK_APPLICATIONS[idx] = next;
  return next;
}

export async function getQueueCounts(): Promise<QueueCounts> {
  const counts: QueueCounts = {
    verification: 0,
    moderation: 0,
    contracts: 0,
    creators: 0,
    rejected: 0,
  };
  for (const app of MOCK_APPLICATIONS) {
    counts[app.stage] += 1;
  }
  return delay(counts);
}

export async function listApplications(stage: ApplicationStage): Promise<Application[]> {
  return delay(MOCK_APPLICATIONS.filter((a) => a.stage === stage));
}

export async function getApplication(id: string): Promise<Application | undefined> {
  return delay(MOCK_APPLICATIONS.find((a) => a.id === id));
}

export async function approveApplication(id: string): Promise<void> {
  patchApplication(id, {
    stage: "contracts",
    approvedAt: new Date().toISOString(),
    contractStatus: "not_sent",
  });
  return delay(undefined);
}

export async function sendContracts(ids: string[]): Promise<void> {
  const now = new Date().toISOString();
  for (const id of ids) {
    const idx = MOCK_APPLICATIONS.findIndex((a) => a.id === id);
    if (idx < 0) continue;
    const current = MOCK_APPLICATIONS[idx];
    if (!current || current.stage !== "contracts") continue;
    MOCK_APPLICATIONS[idx] = {
      ...current,
      contractStatus: "sent",
      updatedAt: now,
    };
  }
  return delay(undefined);
}

export async function revokeContract(id: string): Promise<void> {
  const current = MOCK_APPLICATIONS.find((a) => a.id === id);
  if (!current) throw new Error("Application not found");
  if (current.stage !== "contracts" || current.contractStatus !== "sent") {
    throw new Error("Contract is not in 'sent' state");
  }
  patchApplication(id, {
    contractStatus: "not_sent",
    updatedAt: new Date().toISOString(),
  });
  return delay(undefined);
}

export async function rejectApplication(
  id: string,
  comment?: string,
): Promise<void> {
  const trimmed = comment?.trim();
  patchApplication(id, {
    stage: "rejected",
    rejectedAt: new Date().toISOString(),
    ...(trimmed ? { rejectionComment: trimmed } : {}),
  });
  return delay(undefined);
}

export async function returnToModeration(id: string): Promise<void> {
  const current = MOCK_APPLICATIONS.find((a) => a.id === id);
  if (!current) throw new Error("Application not found");
  const idx = MOCK_APPLICATIONS.findIndex((a) => a.id === id);
  const next = { ...current, stage: "moderation" as const };
  delete next.approvedAt;
  delete next.contractStatus;
  MOCK_APPLICATIONS[idx] = next;
  return delay(undefined);
}

export async function saveInternalNote(
  id: string,
  note: string,
): Promise<void> {
  const trimmed = note.trim();
  const current = MOCK_APPLICATIONS.find((a) => a.id === id);
  if (!current) throw new Error("Application not found");
  const idx = MOCK_APPLICATIONS.findIndex((a) => a.id === id);
  if (trimmed) {
    MOCK_APPLICATIONS[idx] = { ...current, internalNote: trimmed };
  } else {
    const next = { ...current };
    delete next.internalNote;
    MOCK_APPLICATIONS[idx] = next;
  }
  return delay(undefined);
}
