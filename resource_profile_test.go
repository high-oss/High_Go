// Copyright (c) 2026 Truestock
// SPDX-License-Identifier: MIT

package highopenapi_test

import (
	"context"
	"net/http"
	"testing"

	highopenapi "github.com/high-oss/High_Go"
)

func TestProfileGetReturnsTheUnwrappedProfile(t *testing.T) {
	ts := highopenapi.StartTestServer(func(w http.ResponseWriter, r *http.Request, _ int) {
		highopenapi.WriteJSON(w, 200, highopenapi.Envelope("r", map[string]any{
			"clientId": "HS1234", "name": "Asha Verma", "email": "asha.verma@example.com", "mobile": "9876543210",
			"pan": "XXXXXX234F", "aadhaar": "XXXXXXXX9012", "depository": "CDSL", "boCode": "1208160000123456",
			"dpCode": "12081600", "isPOAApproved": true, "isNRI": false, "isMTFEnabled": true,
			"segmentsAllowed": []string{"NSE_CASH", "NSE_FNO"},
			"address":         map[string]any{"address": "12, MG Road", "city": "Bengaluru", "state": "Karnataka", "pinCode": "560001"},
			"bankAccounts": []map[string]any{
				{"bankName": "HDFC Bank", "accountNumber": "XXXXXX4321", "ifscCode": "HDFC0000001", "branch": "MG Road", "isPrimary": true},
			},
		}))
	})
	defer ts.Close()

	profile, err := clientFor(t, ts, nil).Profile.Get(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if profile.ClientId != "HS1234" || !profile.IsMTFEnabled || profile.IsNRI {
		t.Errorf("profile = %+v", profile)
	}
	if profile.Address.PinCode != "560001" {
		t.Errorf("Address.PinCode = %q", profile.Address.PinCode)
	}
	if len(profile.BankAccounts) != 1 || !profile.BankAccounts[0].IsPrimary || profile.BankAccounts[0].AccountNumber != "XXXXXX4321" {
		t.Errorf("BankAccounts = %+v", profile.BankAccounts)
	}
	req := ts.Req(0)
	if req.Method != "GET" || req.URL != "/v1/profile" {
		t.Errorf("request = %s %s", req.Method, req.URL)
	}
}
