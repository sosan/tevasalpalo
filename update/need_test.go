package update

import "testing"

func Test_cleanRawChecksum(t *testing.T) {
	type args struct {
		checksumRaw string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: `sin retorno al final \n`,
			args: args{
				checksumRaw: "b5175c5fadda1366b4b0e0d8a83bfdde110fd2759025b8cf6112885b257edd79",
			},
			want: "b5175c5fadda1366b4b0e0d8a83bfdde110fd2759025b8cf6112885b257edd79",
		},
		{
			name: `con retorno al final \n`,
			args: args{
				checksumRaw: "b5175c5fadda1366b4b0e0d8a83bfdde110fd2759025b8cf6112885b257edd79\n",
			},
			want: "b5175c5fadda1366b4b0e0d8a83bfdde110fd2759025b8cf6112885b257edd79",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := cleanRawDate(tt.args.checksumRaw); got != tt.want {
				t.Errorf("cleanRawChecksum() = %v, want %v", got, tt.want)
			}
		})
	}
}
