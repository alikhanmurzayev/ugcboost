import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { useSearchParams } from "react-router-dom";
import {
  returnToModeration,
  revokeContract,
  sendContracts,
} from "@/_prototype/api/creatorApplications";
import { creatorApplicationKeys } from "@/_prototype/queryKeys";
import { ApiError } from "@/_prototype/api/client";
import { getErrorMessage } from "@/_prototype/shared/i18n/errors";
import type { Application } from "../types";

interface Props {
  application: Application;
}

export default function ContractsActions({ application }: Props) {
  const { t } = useTranslation("prototype_creatorApplications");
  const queryClient = useQueryClient();
  const [, setSearchParams] = useSearchParams();
  const [error, setError] = useState("");

  const isSent = application.contractStatus === "sent";
  const canSend = application.contractStatus === "not_sent";

  const sendMut = useMutation({
    mutationFn: () => sendContracts([application.id]),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: creatorApplicationKeys.all() });
    },
    onError(err) {
      setError(
        err instanceof ApiError
          ? getErrorMessage(err.code)
          : t("errors.sendContractError"),
      );
    },
  });

  const revokeMut = useMutation({
    mutationFn: () => revokeContract(application.id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: creatorApplicationKeys.all() });
    },
    onError(err) {
      setError(
        err instanceof ApiError
          ? getErrorMessage(err.code)
          : t("errors.revokeContractError"),
      );
    },
  });

  const returnMut = useMutation({
    mutationFn: () => returnToModeration(application.id),
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: creatorApplicationKeys.all() });
      setSearchParams((prev) => {
        const np = new URLSearchParams(prev);
        np.delete("id");
        return np;
      });
    },
    onError(err) {
      setError(
        err instanceof ApiError
          ? getErrorMessage(err.code)
          : t("errors.returnError"),
      );
    },
  });

  const isPending =
    sendMut.isPending || revokeMut.isPending || returnMut.isPending;

  return (
    <div data-testid="contracts-actions">
      <div className="flex flex-wrap gap-2">
        {isSent ? (
          <button
            type="button"
            onClick={() => revokeMut.mutate()}
            disabled={isPending}
            className="rounded-button bg-red-600 px-4 py-2 text-sm font-semibold text-white transition hover:bg-red-700 disabled:opacity-50"
            data-testid="revoke-contract-button"
          >
            {revokeMut.isPending
              ? t("actions.revoking")
              : t("actions.revokeContract")}
          </button>
        ) : (
          <button
            type="button"
            onClick={() => sendMut.mutate()}
            disabled={!canSend || isPending}
            className="rounded-button bg-emerald-600 px-4 py-2 text-sm font-semibold text-white transition hover:bg-emerald-700 disabled:opacity-50"
            data-testid="send-contract-button"
          >
            {sendMut.isPending
              ? t("actions.sending")
              : t("actions.sendContract")}
          </button>
        )}
        {!isSent && (
          <button
            type="button"
            onClick={() => returnMut.mutate()}
            disabled={isPending}
            className="rounded-button border border-amber-600 px-4 py-2 text-sm font-semibold text-amber-700 transition hover:bg-amber-50 disabled:opacity-50"
            data-testid="return-to-moderation"
          >
            {returnMut.isPending
              ? t("actions.returning")
              : t("actions.returnToModeration")}
          </button>
        )}
      </div>
      {error && (
        <p className="mt-2 text-sm text-red-600" role="alert">
          {error}
        </p>
      )}
    </div>
  );
}
