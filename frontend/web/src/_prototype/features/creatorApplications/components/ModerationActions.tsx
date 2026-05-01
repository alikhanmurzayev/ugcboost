import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { useSearchParams } from "react-router-dom";
import {
  approveApplication,
  rejectApplication,
} from "@/_prototype/api/creatorApplications";
import { creatorApplicationKeys } from "@/_prototype/queryKeys";
import Modal from "@/_prototype/shared/components/Modal";
import { ApiError } from "@/_prototype/api/client";
import { getErrorMessage } from "@/_prototype/shared/i18n/errors";
import type { Application } from "../types";

interface ModerationActionsProps {
  application: Application;
}

export default function ModerationActions({ application }: ModerationActionsProps) {
  const { t } = useTranslation("prototype_creatorApplications");
  const queryClient = useQueryClient();
  const [, setSearchParams] = useSearchParams();
  const [rejectOpen, setRejectOpen] = useState(false);
  const [rejectComment, setRejectComment] = useState("");
  const [error, setError] = useState("");

  function closeDrawerInUrl() {
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev);
      next.delete("id");
      return next;
    });
  }

  function invalidateAll() {
    queryClient.invalidateQueries({ queryKey: creatorApplicationKeys.all() });
  }

  const approveMut = useMutation({
    mutationFn: () => approveApplication(application.id),
    onSuccess: () => {
      invalidateAll();
      closeDrawerInUrl();
    },
    onError(err) {
      setError(
        err instanceof ApiError
          ? getErrorMessage(err.code)
          : t("errors.approveError"),
      );
    },
  });

  const rejectMut = useMutation({
    mutationFn: (comment: string) =>
      rejectApplication(application.id, comment || undefined),
    onSuccess: () => {
      setRejectOpen(false);
      setRejectComment("");
      invalidateAll();
      closeDrawerInUrl();
    },
    onError(err) {
      setError(
        err instanceof ApiError
          ? getErrorMessage(err.code)
          : t("errors.rejectError"),
      );
    },
  });

  return (
    <>
      <div className="flex flex-wrap gap-2" data-testid="moderation-actions">
        <button
          type="button"
          disabled={approveMut.isPending}
          onClick={() => approveMut.mutate()}
          className="rounded-button bg-emerald-600 px-4 py-2 text-sm font-semibold text-white transition hover:bg-emerald-700 disabled:opacity-50"
          data-testid="approve-button"
        >
          {approveMut.isPending
            ? t("actions.approving")
            : t("actions.approve")}
        </button>
        <button
          type="button"
          disabled={approveMut.isPending}
          onClick={() => setRejectOpen(true)}
          className="rounded-button border border-red-600 px-4 py-2 text-sm font-semibold text-red-600 transition hover:bg-red-50 disabled:opacity-50"
          data-testid="reject-button"
        >
          {t("actions.reject")}
        </button>
      </div>
      {error && (
        <p className="mt-2 text-sm text-red-600" role="alert">
          {error}
        </p>
      )}

      <Modal
        open={rejectOpen}
        onClose={() => !rejectMut.isPending && setRejectOpen(false)}
        title={t("rejectDialog.title")}
        footer={
          <>
            <button
              type="button"
              onClick={() => setRejectOpen(false)}
              disabled={rejectMut.isPending}
              className="rounded-button px-4 py-2 text-sm text-gray-600 hover:bg-surface-200 disabled:opacity-50"
              data-testid="reject-cancel"
            >
              {t("rejectDialog.cancel")}
            </button>
            <button
              type="button"
              onClick={() => rejectMut.mutate(rejectComment)}
              disabled={rejectMut.isPending}
              className="rounded-button bg-red-600 px-4 py-2 text-sm font-semibold text-white transition hover:bg-red-700 disabled:opacity-50"
              data-testid="reject-submit"
            >
              {rejectMut.isPending
                ? t("actions.rejecting")
                : t("rejectDialog.submit")}
            </button>
          </>
        }
      >
        <label
          htmlFor="reject-comment"
          className="block text-sm font-medium text-gray-700"
        >
          {t("rejectDialog.comment")}{" "}
          <span className="text-gray-400">
            ({t("rejectDialog.commentOptional")})
          </span>
        </label>
        <textarea
          id="reject-comment"
          value={rejectComment}
          onChange={(e) => setRejectComment(e.target.value)}
          rows={4}
          className="mt-2 w-full rounded-button border border-gray-300 px-3 py-2 text-sm outline-none transition focus:border-primary focus:ring-2 focus:ring-primary-100"
          data-testid="reject-comment"
        />
        <p className="mt-2 text-xs text-gray-500">
          {t("rejectDialog.commentHint")}
        </p>
      </Modal>
    </>
  );
}
