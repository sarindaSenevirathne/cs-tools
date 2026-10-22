// Copyright (c) 2026 WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

import { Button, CircularProgress, Menu, MenuItem, Tooltip } from "@wso2/oxygen-ui";
import { ChevronDown, Download } from "@wso2/oxygen-ui-icons-react";
import { useId, useState, type JSX } from "react";
import { useErrorBanner } from "@context/error-banner/ErrorBannerContext";
import { useSuccessBanner } from "@context/success-banner/SuccessBannerContext";
import { useFilteredListExport, type UseFilteredListExportConfig } from "@hooks/useFilteredListExport";
import { LIST_EXPORT_FORMATS, type ListExportFormat } from "@utils/listExport";

export interface FilteredExportMenuProps<T> extends UseFilteredListExportConfig<T> {
  /** Disable the action entirely (e.g. the list query errored). */
  disabled?: boolean;
  /** Noun used in banner copy, e.g. "incidents". Defaults to `entityName` with hyphens as spaces. */
  entityNounPlural?: string;
}

/**
 * "Export" for a filtered list: a menu of CSV, Excel (.xlsx), JSON and PDF. Each
 * exports the whole filtered (and sorted) result set, not the page on screen,
 * with the same live progress and the same plain notice when the safety cap
 * cut it short as `FilteredCsvExportButton`.
 */
export default function FilteredExportMenu<T>({
  disabled,
  entityNounPlural,
  ...config
}: FilteredExportMenuProps<T>): JSX.Element {
  const { isExporting, progress, exportingFormat, runExport } = useFilteredListExport(config);
  const { showError } = useErrorBanner();
  const { showSuccess } = useSuccessBanner();
  const [anchor, setAnchor] = useState<HTMLElement | null>(null);
  const menuId = useId();
  const noun = entityNounPlural ?? config.entityName.replace(/-/g, " ");

  const exportAs = async (format: ListExportFormat): Promise<void> => {
    setAnchor(null);
    const label = LIST_EXPORT_FORMATS.find((f) => f.format === format)?.label ?? format;
    try {
      const { truncated, rowCount, total } = await runExport(format);
      if (truncated) {
        showError(
          `Export stopped at ${rowCount.toLocaleString()} of ${total.toLocaleString()} ${noun} — the file only contains the first ${rowCount.toLocaleString()}. Narrow your filters and export again to get the rest.`,
        );
      } else {
        showSuccess(`Exported ${rowCount.toLocaleString()} ${noun} to ${label}.`);
      }
    } catch (err) {
      showError(
        "Could not complete the export. No file was downloaded — please try again, or narrow your filters first.",
        err,
      );
    }
  };

  const formatLabel = LIST_EXPORT_FORMATS.find((f) => f.format === exportingFormat)?.label;
  const label =
    isExporting && progress
      ? progress.total > 0
        ? `Exporting ${progress.loaded.toLocaleString()} of ${progress.total.toLocaleString()}…`
        : `Exporting${formatLabel ? ` ${formatLabel}` : ""}…`
      : "Export";

  const button = (
    <Button
      size="small"
      variant="text"
      startIcon={isExporting ? <CircularProgress size={14} /> : <Download size={14} />}
      endIcon={isExporting ? undefined : <ChevronDown size={14} />}
      disabled={disabled || isExporting}
      onClick={(e) => setAnchor(e.currentTarget)}
      aria-busy={isExporting}
      aria-haspopup="menu"
      aria-expanded={!!anchor}
      aria-controls={anchor ? menuId : undefined}
    >
      {label}
    </Button>
  );

  return (
    <>
      {isExporting ? (
        <Tooltip title="Fetching every page of the current filter before downloading — this can take a while for a wide filter.">
          <span>{button}</span>
        </Tooltip>
      ) : (
        button
      )}
      <Menu
        anchorEl={anchor}
        open={!!anchor}
        onClose={() => setAnchor(null)}
        MenuListProps={{ id: menuId, "aria-label": "Export format" }}
      >
        {LIST_EXPORT_FORMATS.map(({ format, label: itemLabel }) => (
          <MenuItem key={format} onClick={() => void exportAs(format)}>
            {itemLabel}
          </MenuItem>
        ))}
      </Menu>
    </>
  );
}
