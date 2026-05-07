import { useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import {
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import Drawer from "@/shared/components/Drawer";
import Spinner from "@/shared/components/Spinner";
import ErrorState from "@/shared/components/ErrorState";
import { ApiError } from "@/api/client";
import {
  addCampaignCreators,
  type CampaignCreator,
} from "@/api/campaignCreators";
import { listCreators } from "@/api/creators";
import {
  campaignCreatorKeys,
  campaignKeys,
  creatorKeys,
} from "@/shared/constants/queryKeys";
import {
  isFilterActive,
  toListInput,
  type FilterValues,
} from "@/features/creators/filters";
import type { SortState } from "@/features/creators/sort";
import DrawerCreatorFilters from "./DrawerCreatorFilters";
import AddCreatorsDrawerTable from "./AddCreatorsDrawerTable";
import { useDrawerSelection } from "./hooks/useDrawerSelection";

interface AddCreatorsDrawerProps {
  open: boolean;
  campaignId: string;
  existingCreatorIds: Set<string>;
  onClose: () => void;
  onAdded?: (added: CampaignCreator[]) => void;
  cap?: number;
}

const PER_PAGE = 50;
const DRAWER_DEFAULT_SORT: SortState = { sort: "created_at", order: "desc" };

const EMPTY_FILTERS: FilterValues = {
  search: undefined,
  dateFrom: undefined,
  dateTo: undefined,
  cities: [],
  ageFrom: undefined,
  ageTo: undefined,
  categories: [],
};

export default function AddCreatorsDrawer({
  open,
  campaignId,
  existingCreatorIds,
  onClose,
  onAdded,
  cap,
}: AddCreatorsDrawerProps) {
  const { t } = useTranslation("campaigns");
  const { t: tCreators } = useTranslation("creators");
  const { t: tCommon } = useTranslation("common");
  const queryClient = useQueryClient();

  const [filters, setFilters] = useState<FilterValues>(EMPTY_FILTERS);
  const [sort, setSort] = useState<SortState>(DRAWER_DEFAULT_SORT);
  const [page, setPage] = useState(1);
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const [isSubmitting, setIsSubmitting] = useState(false);

  const { selected, size, capReached, toggle, clear } = useDrawerSelection(cap);

  const onCloseRef = useRef(onClose);
  useEffect(() => {
    onCloseRef.current = onClose;
  }, [onClose]);

  const isMountedRef = useRef(true);
  useEffect(() => {
    return () => {
      isMountedRef.current = false;
    };
  }, []);

  function resetDrawerState() {
    setFilters(EMPTY_FILTERS);
    setSort(DRAWER_DEFAULT_SORT);
    setPage(1);
    setErrorMessage(null);
    setIsSubmitting(false);
    clear();
  }

  const listInput = useMemo(
    () => toListInput(filters, { sort, page, perPage: PER_PAGE }),
    [filters, sort, page],
  );

  const listQuery = useQuery({
    queryKey: creatorKeys.list(listInput),
    queryFn: () => listCreators(listInput),
    enabled: open,
  });

  const items = listQuery.data?.data?.items ?? [];
  const total = listQuery.data?.data?.total ?? 0;
  const totalPages = Math.max(1, Math.ceil(total / PER_PAGE));
  // External changes (other tab added creators, filter shrank result) can
  // leave `page` past `totalPages`. We can't clamp via setState in an effect
  // (forbidden by react-hooks/set-state-in-effect), so we display and click-
  // gate against this clamped value; the next prev/next click rolls state
  // back into range.
  const displayPage = Math.min(page, totalPages);

  const addMutation = useMutation({
    mutationFn: () => addCampaignCreators(campaignId, [...selected]),
    onSuccess: (added) => {
      void queryClient.invalidateQueries({
        queryKey: campaignCreatorKeys.list(campaignId),
      });
      onAdded?.(added);
      if (!isMountedRef.current) return;
      resetDrawerState();
      onCloseRef.current();
    },
    onError: (err) => {
      if (!isMountedRef.current) return;
      const apiErr = err instanceof ApiError ? err : null;
      if (
        apiErr?.status === 422 &&
        apiErr.code === "CREATOR_ALREADY_IN_CAMPAIGN"
      ) {
        // Spec: invalidate + alert; selection survives. Conflicting ids will
        // surface as «Добавлен» (disabled checkbox) on the next render once
        // existingCreatorIds refreshes — no need to clear all picks.
        void queryClient.invalidateQueries({
          queryKey: campaignCreatorKeys.list(campaignId),
        });
        setErrorMessage(t("campaignCreators.errors.alreadyInCampaign"));
        return;
      }
      if (apiErr?.status === 404) {
        // The campaign was soft-deleted between fetch and submit. Parent
        // refetch via campaignKeys.detail will flip isDeleted=true and the
        // section unmounts (Spec B rule), so an inline alert here would
        // never be seen anyway — close silently.
        void queryClient.invalidateQueries({
          queryKey: campaignKeys.detail(campaignId),
        });
        resetDrawerState();
        onCloseRef.current();
        return;
      }
      // Other 422s carry server-side validation messages (CAMPAIGN_CREATOR_IDS_REQUIRED,
      // CREATOR_NOT_FOUND, ...). Surface the server's user-facing message when present;
      // fall back to the generic «addFailed» otherwise.
      if (apiErr?.status === 422 && apiErr.serverMessage) {
        setErrorMessage(apiErr.serverMessage);
        return;
      }
      setErrorMessage(t("campaignCreators.errors.addFailed"));
    },
    onSettled: () => {
      if (!isMountedRef.current) return;
      setIsSubmitting(false);
    },
  });

  function handleFiltersChange(next: FilterValues) {
    setFilters(next);
    setPage(1);
  }

  function handleSortChange(columnKey: string) {
    const field = mapColumnToSortField(columnKey);
    if (!field) return;
    setSort((prev) =>
      prev.sort === field
        ? { sort: field, order: prev.order === "asc" ? "desc" : "asc" }
        : { sort: field, order: "asc" },
    );
    setPage(1);
  }

  function handleSubmit() {
    if (size === 0 || isSubmitting || addMutation.isPending) return;
    setIsSubmitting(true);
    setErrorMessage(null);
    addMutation.mutate();
  }

  function handleCancel() {
    if (isSubmitting || addMutation.isPending) return;
    resetDrawerState();
    onClose();
  }

  const submitDisabled =
    size === 0 || isSubmitting || addMutation.isPending;

  return (
    <Drawer
      open={open}
      onClose={handleCancel}
      title={t("campaignCreators.addDrawerTitle")}
      widthClassName="w-[1100px] max-w-[90vw]"
    >
      <div data-testid="add-creators-drawer-body">
        <DrawerCreatorFilters filters={filters} onChange={handleFiltersChange} />

        {errorMessage && (
          <p
            className="mt-4 rounded-button border border-red-200 bg-red-50 px-3 py-2 text-sm text-red-700"
            role="alert"
            data-testid="add-creators-drawer-error"
          >
            {errorMessage}
          </p>
        )}

        {listQuery.isLoading ? (
          <Spinner className="mt-6" />
        ) : listQuery.isError ? (
          <ErrorState
            message={tCreators("loadError")}
            onRetry={() => void listQuery.refetch()}
          />
        ) : (
          <>
            <AddCreatorsDrawerTable
              rows={items}
              selected={selected}
              existingCreatorIds={existingCreatorIds}
              capReached={capReached}
              onToggle={toggle}
              sortColumn={mapSortFieldToColumn(sort.sort)}
              sortOrder={sort.order}
              onSortChange={handleSortChange}
              emptyMessage={
                isFilterActive(filters)
                  ? tCreators("emptyFiltered")
                  : t("campaignCreators.drawerEmpty")
              }
            />

            {totalPages > 1 && (
              <div
                className="mt-4 flex items-center justify-between"
                data-testid="add-creators-drawer-pagination"
              >
                <button
                  type="button"
                  onClick={() =>
                    setPage(Math.max(1, Math.min(totalPages, page) - 1))
                  }
                  disabled={displayPage <= 1}
                  className="rounded-button border border-surface-300 bg-white px-3 py-1.5 text-sm font-medium text-gray-700 transition hover:bg-surface-100 disabled:cursor-not-allowed disabled:opacity-50"
                  data-testid="add-creators-drawer-pagination-prev"
                >
                  {tCommon("prev")}
                </button>
                <span
                  className="text-sm text-gray-500"
                  data-testid="add-creators-drawer-pagination-info"
                >
                  {tCreators("pagination.page", {
                    page: displayPage,
                    total: totalPages,
                  })}
                </span>
                <button
                  type="button"
                  onClick={() =>
                    setPage(Math.min(totalPages, Math.min(totalPages, page) + 1))
                  }
                  disabled={displayPage >= totalPages}
                  className="rounded-button border border-surface-300 bg-white px-3 py-1.5 text-sm font-medium text-gray-700 transition hover:bg-surface-100 disabled:cursor-not-allowed disabled:opacity-50"
                  data-testid="add-creators-drawer-pagination-next"
                >
                  {tCommon("next")}
                </button>
              </div>
            )}
          </>
        )}

        <div className="mt-6 flex items-center justify-between border-t border-surface-200 pt-4">
          <div
            className={`text-sm font-medium ${
              capReached ? "text-amber-600" : "text-gray-700"
            }`}
            data-testid="add-creators-drawer-counter"
          >
            {t("campaignCreators.capCounter", { count: size })}
            {capReached && (
              <span
                className="ml-2 text-xs font-normal text-amber-700"
                data-testid="add-creators-drawer-cap-hint"
              >
                {t("campaignCreators.capHint")}
              </span>
            )}
          </div>
          <div className="flex gap-2">
            <button
              type="button"
              onClick={handleCancel}
              disabled={isSubmitting || addMutation.isPending}
              className="rounded-button border border-surface-300 bg-white px-3 py-1.5 text-sm font-medium text-gray-700 transition hover:bg-surface-100 disabled:cursor-not-allowed disabled:opacity-50"
              data-testid="add-creators-drawer-cancel"
            >
              {t("campaignCreators.cancelButton")}
            </button>
            <button
              type="button"
              onClick={handleSubmit}
              disabled={submitDisabled}
              className="rounded-button bg-primary px-3 py-1.5 text-sm font-medium text-white transition hover:bg-primary-700 disabled:cursor-not-allowed disabled:opacity-50"
              data-testid="add-creators-drawer-submit"
            >
              {isSubmitting || addMutation.isPending
                ? t("campaignCreators.addSubmittingButton")
                : t("campaignCreators.addSubmitButton", { count: size })}
            </button>
          </div>
        </div>
      </div>
    </Drawer>
  );
}

function mapColumnToSortField(columnKey: string): SortState["sort"] | undefined {
  switch (columnKey) {
    case "fullName":
      return "full_name";
    case "age":
      return "birth_date";
    case "city":
      return "city_name";
    case "createdAt":
      return "created_at";
    default:
      return undefined;
  }
}

function mapSortFieldToColumn(field: SortState["sort"]): string | undefined {
  switch (field) {
    case "full_name":
      return "fullName";
    case "birth_date":
      return "age";
    case "city_name":
      return "city";
    case "created_at":
      return "createdAt";
    default:
      return undefined;
  }
}
