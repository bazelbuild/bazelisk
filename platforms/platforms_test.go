package platforms

import "testing"

func TestDarwinFallback(t *testing.T) {
	type args struct {
		machineName string
		version     string
	}
	tests := []struct {
		name                 string
		args                 args
		wantAlterMachineName string
		wantErr              bool
	}{
		{
			name: "before 4.1.0, x86_64 do not fallback",
			args: args{
				machineName: "x86_64",
				version:     "4.0.1",
			},
			wantAlterMachineName: "x86_64",
			wantErr:              false,
		},
		{
			name: "since 4.1.0, x86_64 do not fallback either",
			args: args{
				machineName: "x86_64",
				version:     "4.1.0",
			},
			wantAlterMachineName: "x86_64",
			wantErr:              false,
		},
		{
			name: "before 4.1.0, arm64 not supported, fallback to x86_64 on arm64",
			args: args{
				machineName: "arm64",
				version:     "4.0.1",
			},
			wantAlterMachineName: "x86_64",
			wantErr:              false,
		},
		{
			name: "since 4.1.0, arm64 supported, do not fallback",
			args: args{
				machineName: "arm64",
				version:     "4.1.0",
			},
			wantAlterMachineName: "arm64",
			wantErr:              false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotAlterMachineName, err := DarwinFallback(tt.args.machineName, tt.args.version)
			if (err != nil) != tt.wantErr {
				t.Errorf("DarwinFallback() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if gotAlterMachineName != tt.wantAlterMachineName {
				t.Errorf("DarwinFallback() gotAlterMachineName = %v, want %v", gotAlterMachineName, tt.wantAlterMachineName)
			}
		})
	}
}
