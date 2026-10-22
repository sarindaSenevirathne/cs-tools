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

import { Alert, Box, Typography } from "@wso2/oxygen-ui";
import { useEffect, useRef, type JSX } from "react";
import { bulkResultSummary, type BulkResult } from "@features/csm-operations/utils/bulkUpdate";

interface BulkResultAlertProps {
  result: BulkResult;
  /** e.g. "incidents" (plural). */
  nounPlural: string;
  verb?: string;
  onClose: () => void;
}

/**
 * The outcome of a bulk action over a list: how many records were updated and,
 * for each one that was not, its number and the reason in the backend's words
 * (a refused state move, a field an approved change request no longer takes),
 * so the engineer knows exactly which records still need attention.
 *
 * It takes focus when it appears: the button that started the run has gone
 * with the selection bar, and moving focus here also has a screen reader read
 * the outcome (a status region inserted with its text is not reliably announced).
 */
export default function BulkResultAlert({ result, nounPlural, verb, onClose }: BulkResultAlertProps): JSX.Element {
  const problems = [...result.failed, ...result.skipped];
  const severity = result.succeeded.length === 0 ? "error" : problems.length > 0 ? "warning" : "success";
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    ref.current?.focus();
  }, [result]);
  return (
    <Alert
      ref={ref}
      tabIndex={-1}
      severity={severity}
      onClose={onClose}
      role={problems.length > 0 ? "alert" : "status"}
      sx={{ "&:focus-visible": { outline: 2, outlineColor: "primary.main", outlineOffset: 2 } }}
    >
      <Typography variant="body2" sx={{ fontWeight: 600 }}>
        {bulkResultSummary(result, nounPlural, verb)}
      </Typography>
      {problems.length > 0 && (
        <Box component="ul" sx={{ m: 0, mt: 0.5, pl: 2.5 }} aria-label="Not updated">
          {problems.map((p) => (
            <li key={p.id}>
              <Typography variant="body2" component="span">
                <strong>{p.number}</strong>: {p.message}
              </Typography>
            </li>
          ))}
        </Box>
      )}
    </Alert>
  );
}
