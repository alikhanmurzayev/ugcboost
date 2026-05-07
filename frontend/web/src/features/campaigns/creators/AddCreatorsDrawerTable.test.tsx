import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import "@/shared/i18n/config";
import type { CreatorListItem } from "@/api/creators";
import AddCreatorsDrawerTable from "./AddCreatorsDrawerTable";

const CREATOR_A = "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa";
const CREATOR_B = "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb";
const CREATOR_C = "cccccccc-cccc-cccc-cccc-cccccccccccc";

function makeCreator(id: string, lastName: string): CreatorListItem {
  return {
    id,
    lastName,
    firstName: "Анна",
    middleName: null,
    iin: "070101400001",
    birthDate: "2007-01-01",
    phone: "+77001112255",
    city: { code: "ALA", name: "Алматы", sortOrder: 10 },
    categories: [{ code: "fashion", name: "Мода", sortOrder: 1 }],
    socials: [{ platform: "instagram", handle: lastName.toLowerCase() }],
    telegramUsername: lastName.toLowerCase(),
    createdAt: "2026-04-30T12:00:00Z",
    updatedAt: "2026-04-30T12:00:00Z",
  };
}

const ROWS = [
  makeCreator(CREATOR_A, "Иванова"),
  makeCreator(CREATOR_B, "Сидорова"),
  makeCreator(CREATOR_C, "Петрова"),
];

describe("AddCreatorsDrawerTable", () => {
  it("renders empty message when rows is empty", () => {
    render(
      <AddCreatorsDrawerTable
        rows={[]}
        selected={new Set()}
        existingCreatorIds={new Set()}
        capReached={false}
        onToggle={() => {}}
        emptyMessage="Нет креаторов"
      />,
    );

    expect(
      screen.getByTestId("add-creators-drawer-table-empty"),
    ).toHaveTextContent("Нет креаторов");
  });

  it("renders one row per creator with a checkbox column first", () => {
    render(
      <AddCreatorsDrawerTable
        rows={ROWS}
        selected={new Set()}
        existingCreatorIds={new Set()}
        capReached={false}
        onToggle={() => {}}
        emptyMessage=""
      />,
    );

    expect(
      screen.getByTestId(`drawer-row-checkbox-${CREATOR_A}`),
    ).toBeInTheDocument();
    expect(
      screen.getByTestId(`drawer-row-checkbox-${CREATOR_B}`),
    ).toBeInTheDocument();
    expect(
      screen.getByTestId(`drawer-row-checkbox-${CREATOR_C}`),
    ).toBeInTheDocument();
  });

  it("checkbox is checked when row id is in `selected`", () => {
    render(
      <AddCreatorsDrawerTable
        rows={ROWS}
        selected={new Set([CREATOR_A])}
        existingCreatorIds={new Set()}
        capReached={false}
        onToggle={() => {}}
        emptyMessage=""
      />,
    );

    expect(screen.getByTestId(`drawer-row-checkbox-${CREATOR_A}`)).toBeChecked();
    expect(
      screen.getByTestId(`drawer-row-checkbox-${CREATOR_B}`),
    ).not.toBeChecked();
  });

  it("disabled-row + added badge when creator is in existingCreatorIds; checkbox is disabled", () => {
    render(
      <AddCreatorsDrawerTable
        rows={ROWS}
        selected={new Set()}
        existingCreatorIds={new Set([CREATOR_A])}
        capReached={false}
        onToggle={() => {}}
        emptyMessage=""
      />,
    );

    expect(
      screen.getByTestId(`drawer-row-checkbox-${CREATOR_A}`),
    ).toBeDisabled();
    expect(
      screen.getByTestId(`drawer-row-added-badge-${CREATOR_A}`),
    ).toHaveTextContent("Добавлен");
    expect(
      screen.queryByTestId(`drawer-row-added-badge-${CREATOR_B}`),
    ).not.toBeInTheDocument();
  });

  it("disables unchecked checkboxes when capReached and keeps already-checked enabled", () => {
    render(
      <AddCreatorsDrawerTable
        rows={ROWS}
        selected={new Set([CREATOR_A])}
        existingCreatorIds={new Set()}
        capReached
        onToggle={() => {}}
        emptyMessage=""
      />,
    );

    expect(screen.getByTestId(`drawer-row-checkbox-${CREATOR_A}`)).toBeChecked();
    expect(
      screen.getByTestId(`drawer-row-checkbox-${CREATOR_A}`),
    ).not.toBeDisabled();
    expect(
      screen.getByTestId(`drawer-row-checkbox-${CREATOR_B}`),
    ).toBeDisabled();
    expect(
      screen.getByTestId(`drawer-row-checkbox-${CREATOR_C}`),
    ).toBeDisabled();
  });

  it("calls onToggle(id, isMember=false) when an enabled checkbox is clicked", async () => {
    const onToggle = vi.fn();
    render(
      <AddCreatorsDrawerTable
        rows={ROWS}
        selected={new Set()}
        existingCreatorIds={new Set()}
        capReached={false}
        onToggle={onToggle}
        emptyMessage=""
      />,
    );

    await userEvent.click(
      screen.getByTestId(`drawer-row-checkbox-${CREATOR_A}`),
    );

    expect(onToggle).toHaveBeenCalledTimes(1);
    expect(onToggle).toHaveBeenCalledWith(CREATOR_A, false);
  });

  it("does not call onToggle when a member checkbox is clicked (disabled)", async () => {
    const onToggle = vi.fn();
    render(
      <AddCreatorsDrawerTable
        rows={ROWS}
        selected={new Set()}
        existingCreatorIds={new Set([CREATOR_A])}
        capReached={false}
        onToggle={onToggle}
        emptyMessage=""
      />,
    );

    await userEvent.click(
      screen.getByTestId(`drawer-row-checkbox-${CREATOR_A}`),
    );

    expect(onToggle).not.toHaveBeenCalled();
  });

  it("renders concrete cells for a present creator (fullName, social, category, city)", () => {
    render(
      <AddCreatorsDrawerTable
        rows={[makeCreator(CREATOR_A, "Иванова")]}
        selected={new Set()}
        existingCreatorIds={new Set()}
        capReached={false}
        onToggle={() => {}}
        emptyMessage=""
      />,
    );

    expect(screen.getByText("Иванова Анна")).toBeInTheDocument();
    expect(screen.getByTestId("social-instagram")).toHaveAttribute(
      "href",
      "https://instagram.com/иванова",
    );
    expect(screen.getByText("Мода")).toBeInTheDocument();
    expect(screen.getByText("Алматы")).toBeInTheDocument();
  });
});
