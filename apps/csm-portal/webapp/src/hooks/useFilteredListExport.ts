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

import { useCallback, useRef, useState } from "react";
import { fetchAllPages } from "@utils/csvExport";
import { saveBlob } from "@utils/saveBlob";
import { buildListExportFile, listExportExtension, type ListExportFormat } from "@utils/listExport";
import type { CsvExportProgress, UseFilteredCsvExportConfig } from "@hooks/useFilteredCsvExport";

export interface UseFilteredListExportConfig<T> extends UseFilteredCsvExportConfig<T> {
  /** The list's name, e.g. "Incidents": the PDF title and the Excel sheet. */
  title: string;
}

export interface UseFilteredListExportResult {
  isExporting: boolean;
  progress: CsvExportProgress | null;
  /** The format being exported, while one is. */
  exportingFormat: ListExportFormat | null;
  runExport: (format: ListExportFormat) => Promise<{ truncated: boolean; rowCount: number; total: number }>;
}

/**
 * `useFilteredCsvExport`, in any of the list export formats (CSV, Excel, JSON,
 * PDF): pages the current filtered (and sorted) search to exhaustion or the
 * safety cap, then builds one file of the chosen format. The same rules hold:
 * the whole filtered set, not the page on screen; a capped export is tagged
 * `-partial` in its filename; a failed page downloads nothing.
 */
export function useFilteredListExport<T>(config: UseFilteredListExportConfig<T>): UseFilteredListExportResult {
  const [exportingFormat, setExportingFormat] = useState<ListExportFormat | null>(null);
  const [progress, setProgress] = useState<CsvExportProgress | null>(null);
  const configRef = useRef(config);
  configRef.current = config;

  const runExport = useCallback(async (format: ListExportFormat) => {
    const { header, toRow, entityName, fetchPage, pageSize, maxRows, title } = configRef.current;
    setExportingFormat(format);
    setProgress({ loaded: 0, total: 0 });
    try {
      const { items, truncated, total } = await fetchAllPages(fetchPage, {
        pageSize,
        maxRows,
        onProgress: (loaded, pageTotal) => setProgress({ loaded, total: pageTotal }),
      });
      const rows = items.map(toRow);
      const now = new Date();
      const subtitle = `${rows.length.toLocaleString()}${truncated ? ` of ${total.toLocaleString()}` : ""} ${entityName.replace(/-/g, " ")} · exported ${now.toLocaleString()}`;
      const blob = await buildListExportFile(format, { title, subtitle, header, rows });
      const timestamp = now.toISOString().replace(/[:.]/g, "-");
      saveBlob(blob, `${entityName}-export-${timestamp}${truncated ? "-partial" : ""}.${listExportExtension(format)}`);
      return { truncated, rowCount: rows.length, total };
    } finally {
      setExportingFormat(null);
      setProgress(null);
    }
  }, []);

  return { isExporting: exportingFormat !== null, progress, exportingFormat, runExport };
}
