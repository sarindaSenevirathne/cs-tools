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

import { Box, Button, Chip, Skeleton, Typography } from "@wso2/oxygen-ui";
import { ScrollText, ExternalLink } from "@wso2/oxygen-ui-icons-react";
import { type JSX, useMemo } from "react";
import { useRunbooks } from "@features/csm-operations/api/useRunbooks";
import { findRunbookForIncident } from "@features/csm-operations/utils/runbooks";

interface IncidentRunbookSectionProps {
  subject: string;
}

/** The Overview's Runbook section: the runbook from Operations > Runbooks holding the longest alert name the incident's subject contains. */
export default function IncidentRunbookSection({ subject }: IncidentRunbookSectionProps): JSX.Element {
  const { data: runbooks, isLoading, isError } = useRunbooks();
  const match = useMemo(
    () => (runbooks ? findRunbookForIncident(runbooks, subject) : undefined),
    [runbooks, subject],
  );

  let body: JSX.Element;
  if (isLoading) {
    body = <Skeleton variant="text" width="40%" />;
  } else if (isError) {
    body = (
      <Typography variant="body2" color="text.secondary">
        Could not load runbooks.
      </Typography>
    );
  } else if (!match) {
    body = (
      <Typography variant="body2" color="text.secondary">
        No runbook matches this incident.
      </Typography>
    );
  } else {
    body = (
      <Box
        sx={{
          display: "flex",
          flexWrap: "wrap",
          alignItems: "center",
          justifyContent: "space-between",
          gap: 1.5,
          p: 1.5,
          borderRadius: 1,
          border: 1,
          borderColor: "divider",
          bgcolor: "action.hover",
        }}
      >
        <Box sx={{ display: "flex", alignItems: "flex-start", gap: 1.25, minWidth: 0 }}>
          <Box sx={{ color: "primary.main", display: "flex", pt: 0.25 }}>
            <ScrollText size={20} />
          </Box>
          <Box sx={{ minWidth: 0 }}>
            <Typography variant="body1" sx={{ fontWeight: 600, wordBreak: "break-word" }}>
              {match.runbook.name} ({match.runbook.number})
            </Typography>
            <Typography variant="body2" color="text.secondary" sx={{ wordBreak: "break-word" }}>
              Matched alert: <Box component="code">{match.alertName}</Box>
            </Typography>
          </Box>
        </Box>
        {match.runbook.runbookUrl ? (
          <Button
            variant="contained"
            size="small"
            href={match.runbook.runbookUrl}
            target="_blank"
            rel="noopener noreferrer"
            endIcon={<ExternalLink size={14} />}
          >
            Open runbook
          </Button>
        ) : (
          <Chip size="small" variant="outlined" color="warning" label="No runbook" />
        )}
      </Box>
    );
  }

  return (
    <Box sx={{ display: "flex", flexDirection: "column", gap: 1, pt: 2, borderTop: 1, borderColor: "divider" }}>
      <Typography variant="caption" color="text.secondary" sx={{ textTransform: "uppercase", letterSpacing: 0.4 }}>
        Runbook
      </Typography>
      {body}
    </Box>
  );
}
