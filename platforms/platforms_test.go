package platforms

import "testing"

func TestFormatBazelFilename(t *testing.T) {
	tests := []struct {
		name          string
		version       string
		includeSuffix bool
		osName        string
		machineName   string
		nojdk         bool
		want          string
	}{
		{
			name:          "standard bazel binary",
			version:       "7.1.1",
			includeSuffix: false,
			osName:        "linux",
			machineName:   "x86_64",
			nojdk:         false,
			want:          "bazel-7.1.1-linux-x86_64",
		},
		{
			name:          "nojdk binary",
			version:       "7.1.1",
			includeSuffix: false,
			osName:        "linux",
			machineName:   "x86_64",
			nojdk:         true,
			want:          "bazel_nojdk-7.1.1-linux-x86_64",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := FormatBazelFilename(tt.version, tt.includeSuffix, tt.osName, tt.machineName, tt.nojdk)
			if got != tt.want {
				t.Errorf("FormatBazelFilename() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDarwinFallback(t *testing.T) {
	type args struct {
		machineName string
		version     string
	}
	tests := []struct {
		name                 string
		args                 args
		wantAlterMachineName string
	}{
		{
			name: "before 4.1.0, x86_64 do not fallback",
			args: args{
				machineName: "x86_64",
				version:     "4.0.1",
			},
			wantAlterMachineName: "x86_64",
		},
		{
			name: "since 4.1.0, x86_64 do not fallback either",
			args: args{
				machineName: "x86_64",
				version:     "4.1.0",
			},
			wantAlterMachineName: "x86_64",
		},
		{
			name: "before 4.1.0, arm64 not supported, fallback to x86_64 on arm64",
			args: args{
				machineName: "arm64",
				version:     "4.0.1",
			},
			wantAlterMachineName: "x86_64",
		},
		{
			name: "since 4.1.0, arm64 supported, do not fallback",
			args: args{
				machineName: "arm64",
				version:     "4.1.0",
			},
			wantAlterMachineName: "arm64",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotAlterMachineName := DarwinFallback(tt.args.machineName, tt.args.version)
			if gotAlterMachineName != tt.wantAlterMachineName {
				t.Errorf("DarwinFallback() gotAlterMachineName = %v, want %v", gotAlterMachineName, tt.wantAlterMachineName)
			}
		})
	}
}
