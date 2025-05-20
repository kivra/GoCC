package benchmark

import (
	"github.com/GiGurra/boa/pkg/boa"
	"github.com/kivra/gocc/cmd/benchmark/benchmark_actors"
	"github.com/spf13/cobra"
)

type Cfg struct {
}

func Cmd() *cobra.Command {
	cfg := &Cfg{}
	return boa.Cmd{
		Use:         "benchmark",
		Short:       "run specific benchmarks",
		Params:      cfg,
		ParamEnrich: boa.ParamEnricherDefault,
		SubCmds: []*cobra.Command{
			benchmark_actors.Cmd(),
		},
	}.ToCobra()
}
