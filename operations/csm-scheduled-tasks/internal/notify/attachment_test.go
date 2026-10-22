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

package notify

import (
	"encoding/json"
	"strings"
	"testing"
)

// The email service binds an attachment's content to a byte[] and refuses the
// base64 string encoding/json would write for a []byte, so it travels as an
// array of byte values.
func TestAttachment_ContentIsAnArrayOfBytes(t *testing.T) {
	raw, err := json.Marshal(sendEmailRequest{
		To:          []string{"ann@example.com"},
		Subject:     "s",
		Template:    []byte("<p>hi</p>"),
		Attachments: []Attachment{{ContentName: "invite.ics", ContentType: "text/calendar; method=REQUEST; charset=UTF-8", Content: []byte("BEGIN")}},
	})
	if err != nil {
		t.Fatal(err)
	}
	got := string(raw)
	for _, want := range []string{
		`"attachments":[{"contentName":"invite.ics","contentType":"text/calendar; method=REQUEST; charset=UTF-8","attachment":[66,69,71,73,78]}]`,
		`"template":"PHA+aGk8L3A+"`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("request lacks %s:\n%s", want, got)
		}
	}
	plain, _ := json.Marshal(sendEmailRequest{To: []string{"a@example.com"}, Subject: "s"})
	if strings.Contains(string(plain), "attachments") {
		t.Errorf("an email with no attachment sends the field: %s", plain)
	}
}
