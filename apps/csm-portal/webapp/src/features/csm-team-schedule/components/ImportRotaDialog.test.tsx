/**
 * Copyright (c) 2026 WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { beforeEach, describe, expect, it, vi } from "vitest";
import "@testing-library/jest-dom/vitest";

const postMock = vi.fn();
vi.mock("@api/backend/client", () => ({ useBackendApi: () => ({ post: postMock }) }));
vi.mock("@config/apiConfig", () => ({ apiConfig: { backendUrl: "https://example.test" } }));

import ImportRotaDialog from "./ImportRotaDialog";
import { parseRotaImportFile, rotaImportOffered, ROTA_IMPORT_TEMPLATE } from "../utils/rotaImport";
import type { ImportRotaResult } from "../api/useImportRota";

function fileText(over: Record<string, unknown> = {}): string {
  return JSON.stringify({ ...ROTA_IMPORT_TEMPLATE, ...over });
}

function result(over: Partial<ImportRotaResult> = {}): ImportRotaResult {
  return {
    rotaCode: "SRE_SAAS",
    from: "2026-11-01",
    to: "2026-11-30",
    mode: "replace",
    dryRun: true,
    cleared: 3,
    applied: 17,
    skipped: 2,
    skippedDays: [{ index: 0, dates: ["2026-11-07", "2026-11-08"] }],
    absencesCreated: 1,
    absencesRemoved: 0,
    absencesTrimmed: 0,
    ...over,
  };
}

function renderDialog(rotaCode = "SRE_SAAS", onClose = vi.fn()) {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  render(
    <QueryClientProvider client={qc}>
      <ImportRotaDialog open onClose={onClose} rotaCode={rotaCode} />
    </QueryClientProvider>,
  );
  return { onClose };
}

function chooseFile(text: string, name = "rota.json") {
  fireEvent.change(screen.getByLabelText("Rota file"), {
    target: { files: [new File([text], name, { type: "application/json" })] },
  });
}

describe("parseRotaImportFile", () => {
  it("accepts the template for its own rota, whatever the case of the code", () => {
    expect(parseRotaImportFile(fileText(), "sre_saas").file?.rotaCode).toBe("SRE_SAAS");
  });

  it.each([
    ["not JSON", "{", "The file is not valid JSON."],
    ["a list", "[]", "The file must hold one JSON object."],
    ["another schema version", fileText({ schemaVersion: 2 }), "schemaVersion must be 1."],
    ["no rota", fileText({ rotaCode: "" }), "rotaCode is missing."],
    ["another rota", fileText({ rotaCode: "SRE_IAAS" }), "This file is for SRE_IAAS, but the roster shows SRE_SAAS."],
    ["no period", fileText({ period: { from: "2026-11-01" } }), "period must have from and to dates"],
    ["an unknown mode", fileText({ mode: "upsert" }), 'mode must be "replace" or "merge".'],
    ["assignments not a list", fileText({ assignments: {} }), "assignments must be a list."],
    ["nothing to import", fileText({ assignments: [], absences: [] }), "The file has no assignments and no absences."],
  ])("refuses %s", (_name, text, want) => {
    expect(parseRotaImportFile(text, "SRE_SAAS").error).toContain(want);
  });
});

describe("rotaImportOffered", () => {
  const rotas = [
    { code: "SRE_SAAS", family: "SRE" },
    { code: "SME_MOESIF", family: "SME" },
  ];
  const teams = [
    { key: "apollo", rotaCode: "SRE_SAAS" },
    { key: "moesif", rotaCode: "SME_MOESIF" },
    { key: "castor", rotaCode: null },
  ];

  it("offers it on an SRE rota to a reader who may edit one of its teams", () => {
    expect(rotaImportOffered("SRE_SAAS", rotas, teams, ["Apollo"])).toBe(true);
  });

  it.each([
    ["no rota in scope", undefined, ["apollo"]],
    ["a reader who edits none of its teams", "SRE_SAAS", ["castor"]],
    ["a rota of another family", "SME_MOESIF", ["moesif"]],
    ["a rota the catalogue does not know", "SRE_PAAS", ["apollo"]],
  ])("does not offer it for %s", (_name, rota, editable) => {
    expect(rotaImportOffered(rota, rotas, teams, editable)).toBe(false);
  });
});

describe("ImportRotaDialog", () => {
  beforeEach(() => {
    postMock.mockReset();
  });

  it("previews the chosen file, then imports it", async () => {
    postMock.mockResolvedValueOnce(result()).mockResolvedValueOnce(result({ dryRun: false }));
    renderDialog();
    expect(screen.getByRole("button", { name: "Preview" })).toBeDisabled();

    chooseFile(fileText({ mode: "replace" }));
    expect(await screen.findByText(/4 assignments and 1 absence/)).toBeInTheDocument();
    expect(screen.getByText(/Replace removes every assignment/)).toBeInTheDocument();

    fireEvent.click(screen.getByRole("button", { name: "Preview" }));
    expect(await screen.findByText(/Preview: 17 days on the rota/)).toBeInTheDocument();
    expect(screen.getByText(/3 earlier assignments replaced, 1 absence/)).toBeInTheDocument();
    expect(screen.getByText("assignments[0]: 2026-11-07, 2026-11-08")).toBeInTheDocument();
    expect(postMock).toHaveBeenLastCalledWith(
      "/team-schedule/rotas/SRE_SAAS/import?dryRun=true",
      expect.objectContaining({ rotaCode: "SRE_SAAS", mode: "replace" }),
    );

    fireEvent.click(screen.getByRole("button", { name: "Import" }));
    expect(await screen.findByText(/SRE_SAAS imported: 17 days on the rota/)).toBeInTheDocument();
    expect(postMock).toHaveBeenLastCalledWith("/team-schedule/rotas/SRE_SAAS/import?dryRun=false", expect.anything());
    expect(screen.getByRole("button", { name: "Done" })).toBeInTheDocument();
  });

  it("says a merge keeps the roster, the template's own mode", async () => {
    expect(ROTA_IMPORT_TEMPLATE.mode).toBe("merge");
    renderDialog();
    chooseFile(fileText());
    expect(await screen.findByText(/Merge keeps what is on the roster/)).toBeInTheDocument();
  });

  it("shows the server's refusal, which names the entry at fault", async () => {
    postMock.mockRejectedValueOnce(
      new Error("the rota file has problems: assignments[0]: nobody on the team schedule has the email user1@example.com"),
    );
    renderDialog();
    chooseFile(fileText());
    await screen.findByText(/4 assignments/);
    fireEvent.click(screen.getByRole("button", { name: "Preview" }));
    expect(await screen.findByText(/assignments\[0\]: nobody on the team schedule/)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Import" })).not.toBeInTheDocument();
  });

  it("refuses a file for another rota before anything is sent", async () => {
    renderDialog("SRE_IAAS");
    chooseFile(fileText());
    expect(await screen.findByText(/This file is for SRE_SAAS, but the roster shows SRE_IAAS/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Preview" })).toBeDisabled();
    expect(postMock).not.toHaveBeenCalled();
  });

  it("starts over when another file is chosen", async () => {
    postMock.mockResolvedValueOnce(result());
    renderDialog();
    chooseFile(fileText());
    await screen.findByText(/4 assignments/);
    fireEvent.click(screen.getByRole("button", { name: "Preview" }));
    await screen.findByText(/Preview: 17 days/);

    chooseFile(fileText({ mode: "merge" }), "other.json");
    await waitFor(() => expect(screen.queryByText(/Preview: 17 days/)).not.toBeInTheDocument());
    expect(screen.getByText("other.json")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Preview" })).toBeEnabled();
  });

  it("closes on Cancel", () => {
    const { onClose } = renderDialog();
    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    expect(onClose).toHaveBeenCalled();
  });
});
