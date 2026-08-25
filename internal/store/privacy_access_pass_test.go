package store

import "testing"

func TestNormalizePrivacyAccessPassFee(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "default", input: "", want: "0"},
		{name: "base units", input: " 1000000000000000000 ", want: "1000000000000000000"},
		{name: "canonical zero", input: "000", want: "0"},
		{name: "negative", input: "-1", wantErr: true},
		{name: "decimal", input: "1.5", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := normalizePrivacyAccessPassFee(test.input)
			if (err != nil) != test.wantErr {
				t.Fatalf("normalizePrivacyAccessPassFee() error = %v, wantErr %v", err, test.wantErr)
			}
			if got != test.want {
				t.Fatalf("normalizePrivacyAccessPassFee() = %q, want %q", got, test.want)
			}
		})
	}
}
