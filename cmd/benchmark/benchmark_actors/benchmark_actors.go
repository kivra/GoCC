package benchmark_actors

import (
	"context"
	"fmt"
	"github.com/GiGurra/boa/pkg/boa"
	"github.com/kivra/gocc/pkg/limiter/limiter_api"
	"github.com/kivra/gocc/pkg/limiter/limiter_manager"
	"github.com/kivra/gocc/pkg/logging"
	"github.com/samber/lo"
	lop "github.com/samber/lo/parallel"
	"github.com/spf13/cobra"
	"log/slog"
	"time"
)

type Cfg struct {
}

func Cmd() *cobra.Command {
	cfg := &Cfg{}
	return boa.Cmd{
		Use:         "actors",
		Short:       "run benchmark on internals/actos",
		Params:      cfg,
		ParamEnrich: boa.ParamEnricherDefault,
		RunFunc: func(cmd *cobra.Command, args []string) {
			slog.Info("Running benchmark on internals/actos")
			run1mChannelRequests1Chan10Cons()
			run1mChannelRequests10Chan10Cons()
			run1mChannelRequests100Chan100Cons()
			run1mChannelRequests1000Chan1000Cons()
			run1mChannelRequests10000Chan10000Cons()
			run1mChannelRequests()
			run1mChannelRequestsResponsesNoPipelining()
			run1mChannelRequests10GoRoutinesResponsesNoPipelining()
			run1mReq10ChChannelRequests10GoRoutinesResponsesNoPipelining()
			run1mReq10ChChannelRequests10GoRoutinesResponsesNoPipelining2Jumps()
			runManyLimiterMgrRequests()
		},
	}.ToCobra()
}

func runManyLimiterMgrRequests() {

	numManagers := 25
	nWarmupRequests := 10_000
	nRequests := numManagers * 1_000_000
	nGoRoutines := 100

	slog.Info("********************************************************************")
	slog.Info(fmt.Sprintf(
		"* Running %s requests benchmark on %d limiter_manager(s) with 100 different keys",
		fmtLargeIntIn3DigitSets(int64(nRequests)),
		numManagers,
	))
	logging.ConfigureDefaultLogger("system-default", "warn", false)

	globalCfg := &limiter_api.Config{
		WindowMillis:         10_000,
		MaxRequestsPerWindow: 2 * nRequests,
		MaxRequestsInQueue:   0,
	}

	var debugSnapshot *limiter_api.DebugSnapshotAll
	elapsed := time.Since(time.Now())
	func() { // inner func to allow mgr to close and complete logging here
		mgr := limiter_manager.NewManagerSet(globalCfg, nil, nil, numManagers)
		defer mgr.Close()

		ctx := context.Background()

		runBenchmark := func(nReq int) {
			lop.ForEach(lo.Range(nGoRoutines), func(iact int, _ int) {
				key := fmt.Sprintf("key-%d", iact)
				for i := 0; i < nReq/nGoRoutines; i++ {
					result, _ := mgr.AskPermission(ctx, key, false, limiter_api.NoChange, limiter_api.NoChange)
					if result != limiter_api.Approved {
						panic("Aborting benchmark: expected approved from limiter")
					}
				}
			})
		}

		// warmup
		runBenchmark(nWarmupRequests)

		// actual benchmark
		t0 := time.Now()
		runBenchmark(nRequests)
		elapsed = time.Since(t0)

		debugSnapshot = mgr.GetDebugSnapshotsAll()

	}()

	if elapsed > 30*time.Second {
		panic("Aborting benchmark: took too long, expected < 5s, got " + elapsed.String())
	}

	msgRate := float64(nRequests) / elapsed.Seconds()

	time.Sleep(100 * time.Millisecond) // wait a little to avoid the logs spammed with "stopping limiter mgr"
	logging.ConfigureDefaultLogger("system-default", "info", false)

	expectedNumApproved := (nRequests + nWarmupRequests) / nGoRoutines
	for key, snap := range debugSnapshot.Instances {
		if snap.NumApprovedThisWindow != expectedNumApproved {
			panic(fmt.Sprintf("Aborting benchmark: expected %d approved for %s, got %d", expectedNumApproved, key, snap.NumApprovedThisWindow))
		}
	}

	slog.Info("Benchmark passed")
	slog.Info(fmt.Sprintf("Msg rate: %s msg/s", fmtLargeIntIn3DigitSets(int64(msgRate))))
	slog.Info("Benchmark took", slog.Duration("duration", elapsed))
}

func run1mChannelRequests() {

	slog.Info("********************************************************************")
	slog.Info("* Running 1m requests benchmark on a single channel")
	logging.ConfigureDefaultLogger("system-default", "warn", false)

	nWarmupRequests := 10_000
	nRequests := 1_000_000
	nGoRoutines := 100

	ch := make(chan struct{}, 1_000)

	runBenchmark := func(nReq int) {
		go func() {
			lop.ForEach(lo.Range(nGoRoutines), func(iact int, _ int) {
				for i := 0; i < nReq/nGoRoutines; i++ {
					ch <- struct{}{}
				}
			})
		}()

		for i := 0; i < nReq; i++ {
			<-ch
		}
	}

	// warmup
	runBenchmark(nWarmupRequests)

	// actual benchmark
	t0 := time.Now()
	runBenchmark(nRequests)

	elapsed := time.Since(t0)

	msgRate := float64(nRequests) / elapsed.Seconds()

	logging.ConfigureDefaultLogger("system-default", "info", false)
	slog.Info("Benchmark passed")
	slog.Info(fmt.Sprintf("Msg rate: %s msg/s", fmtLargeIntIn3DigitSets(int64(msgRate))))
	slog.Info("Benchmark took", slog.Duration("duration", elapsed))

}

func run1mChannelRequestsResponsesNoPipelining() {

	slog.Info("********************************************************************")
	slog.Info("* Running 1m requests+response benchmark on a single channel, no pipelining")
	logging.ConfigureDefaultLogger("system-default", "warn", false)

	nWarmupRequests := 100
	nRequests := 1_000_000

	type Response struct {
		// nolint:unused
		lots int
		// nolint:unused
		of int
		// nolint:unused
		data int
		// nolint:unused
		in int
		// nolint:unused
		the int
		// nolint:unused
		response int
	}

	type Request struct {
		// nolint:unused
		lots int
		// nolint:unused
		of int
		// nolint:unused
		data int
		// nolint:unused
		in int
		// nolint:unused
		the int
		// nolint:unused
		request  int
		RespChan chan *Response
	}

	ch := make(chan *Request, 1_000)
	go func() {
		for req := range ch {
			req.RespChan <- &Response{}
		}
	}()
	defer close(ch)

	runBenchmark := func(nReq int) {
		for i := 0; i < nReq; i++ {
			req := &Request{RespChan: make(chan *Response)}
			ch <- req
			<-req.RespChan
		}
	}

	// warmup
	runBenchmark(nWarmupRequests)

	// actual benchmark
	t0 := time.Now()
	runBenchmark(nRequests)
	elapsed := time.Since(t0)

	msgRate := float64(nRequests) / elapsed.Seconds()

	logging.ConfigureDefaultLogger("system-default", "info", false)
	slog.Info("Benchmark passed")
	slog.Info(fmt.Sprintf("Msg rate: %s msg/s", fmtLargeIntIn3DigitSets(int64(msgRate))))
	slog.Info("Benchmark took", slog.Duration("duration", elapsed))

}

func run1mChannelRequests10GoRoutinesResponsesNoPipelining() {

	slog.Info("********************************************************************")
	slog.Info("* Running 1m requests+response with 10 go routines benchmark on a single channel, no pipelining")
	logging.ConfigureDefaultLogger("system-default", "warn", false)

	nWarmupRequests := 100
	nRequests := 1_000_000
	nGoRoutines := 10

	type Response struct {
		// nolint:unused
		lots int
		// nolint:unused
		of int
		// nolint:unused
		data int
		// nolint:unused
		in int
		// nolint:unused
		the int
		// nolint:unused
		response int
	}

	type Request struct {
		// nolint:unused
		lots int
		// nolint:unused
		of int
		// nolint:unused
		data int
		// nolint:unused
		in int
		// nolint:unused
		the int
		// nolint:unused
		request  int
		RespChan chan *Response
	}

	ch := make(chan *Request, 10_000)
	go func() {
		lop.ForEach(lo.Range(nGoRoutines), func(iact int, _ int) {
			for req := range ch {
				req.RespChan <- &Response{}
			}
		})
	}()
	defer close(ch)

	runBenchmark := func(nReq int) {
		lop.ForEach(lo.Range(nGoRoutines), func(iact int, _ int) {
			for i := 0; i < nReq/nGoRoutines; i++ {
				req := &Request{RespChan: make(chan *Response)}
				ch <- req
				<-req.RespChan
			}
		})
	}

	// warmup
	runBenchmark(nWarmupRequests)

	// actual benchmark
	t0 := time.Now()
	runBenchmark(nRequests)
	elapsed := time.Since(t0)

	msgRate := float64(nRequests) / elapsed.Seconds()

	logging.ConfigureDefaultLogger("system-default", "info", false)
	slog.Info("Benchmark passed")
	slog.Info(fmt.Sprintf("Msg rate: %s msg/s", fmtLargeIntIn3DigitSets(int64(msgRate))))
	slog.Info("Benchmark took", slog.Duration("duration", elapsed))

}

func run1mReq10ChChannelRequests10GoRoutinesResponsesNoPipelining() {

	nGR := 8

	slog.Info("********************************************************************")
	slog.Info(fmt.Sprintf("* Running 1m requests+response with %d go routines benchmark on 10 channels, no pipelining", nGR))
	logging.ConfigureDefaultLogger("system-default", "warn", false)

	nWarmupRequests := 100
	nRequests := 1_000_000
	nGoRoutines := nGR

	type Response struct {
		// nolint:unused
		lots int
		// nolint:unused
		of int
		// nolint:unused
		data int
		// nolint:unused
		in int
		// nolint:unused
		the int
		// nolint:unused
		response int
	}

	type Request struct {
		// nolint:unused
		lots int
		// nolint:unused
		of int
		// nolint:unused
		data int
		// nolint:unused
		in int
		// nolint:unused
		the int
		// nolint:unused
		request  int
		RespChan chan *Response
	}

	chs := make([]chan *Request, nGR)
	for i := range chs {
		chs[i] = make(chan *Request, 1_000)
	}
	go func() {
		lop.ForEach(lo.Range(nGoRoutines), func(iGR int, _ int) {
			ch := chs[iGR]
			for req := range ch {
				req.RespChan <- &Response{}
			}
		})
	}()
	defer func() {
		for _, ch := range chs {
			close(ch)
		}
	}()

	runBenchmark := func(nReq int) {
		lop.ForEach(lo.Range(nGoRoutines), func(iGR int, _ int) {
			ch := chs[iGR]
			for i := 0; i < nReq/nGoRoutines; i++ {
				req := &Request{RespChan: make(chan *Response)}
				ch <- req
				<-req.RespChan
			}
		})
	}

	// warmup
	runBenchmark(nWarmupRequests)

	// actual benchmark
	t0 := time.Now()
	runBenchmark(nRequests)
	elapsed := time.Since(t0)

	msgRate := float64(nRequests) / elapsed.Seconds()

	logging.ConfigureDefaultLogger("system-default", "info", false)
	slog.Info("Benchmark passed")
	slog.Info(fmt.Sprintf("Msg rate: %s msg/s", fmtLargeIntIn3DigitSets(int64(msgRate))))
	slog.Info("Benchmark took", slog.Duration("duration", elapsed))

}

func run1mReq10ChChannelRequests10GoRoutinesResponsesNoPipelining2Jumps() {

	nGR := 8

	slog.Info("********************************************************************")
	slog.Info(fmt.Sprintf("* Running 1m requests+response with %d go routines benchmark on 10 channels, no pipelining, 2 jumps", nGR))
	logging.ConfigureDefaultLogger("system-default", "warn", false)

	nWarmupRequests := 100
	nRequests := 1_000_000
	nGoRoutines := nGR

	type Response struct {
		// nolint:unused
		lots int
		// nolint:unused
		of int
		// nolint:unused
		data int
		// nolint:unused
		in int
		// nolint:unused
		the int
		// nolint:unused
		response int
	}

	type Request struct {
		// nolint:unused
		lots int
		// nolint:unused
		of int
		// nolint:unused
		data int
		// nolint:unused
		in int
		// nolint:unused
		the int
		// nolint:unused
		request  int
		RespChan chan *Response
	}

	chsJ1 := make([]chan *Request, nGR)
	for i := range chsJ1 {
		chsJ1[i] = make(chan *Request, 1_000)
	}
	chsJ2 := make([]chan *Request, nGR)
	for i := range chsJ2 {
		chsJ2[i] = make(chan *Request, 1_000)
	}
	go func() {
		lop.ForEach(lo.Range(nGoRoutines), func(iGR int, _ int) {
			srcCh := chsJ1[iGR]
			trgCh := chsJ2[iGR]
			for req := range srcCh {
				trgCh <- req
			}
		})
	}()
	go func() {
		lop.ForEach(lo.Range(nGoRoutines), func(iGR int, _ int) {
			ch := chsJ2[iGR]
			for req := range ch {
				req.RespChan <- &Response{}
			}
		})
	}()
	defer func() {
		for _, ch := range chsJ1 {
			close(ch)
		}
		for _, ch := range chsJ2 {
			close(ch)
		}
	}()

	runBenchmark := func(nReq int) {
		lop.ForEach(lo.Range(nGoRoutines), func(iGR int, _ int) {
			ch := chsJ1[iGR]
			for i := 0; i < nReq/nGoRoutines; i++ {
				req := &Request{RespChan: make(chan *Response)}
				ch <- req
				<-req.RespChan
			}
		})
	}

	// warmup
	runBenchmark(nWarmupRequests)

	// actual benchmark
	t0 := time.Now()
	runBenchmark(nRequests)
	elapsed := time.Since(t0)

	msgRate := float64(nRequests) / elapsed.Seconds()

	logging.ConfigureDefaultLogger("system-default", "info", false)
	slog.Info("Benchmark passed")
	slog.Info(fmt.Sprintf("Msg rate: %s msg/s", fmtLargeIntIn3DigitSets(int64(msgRate))))
	slog.Info("Benchmark took", slog.Duration("duration", elapsed))

}

func run1mChannelRequests1Chan10Cons() {

	slog.Info("********************************************************************")
	slog.Info("* Running 1m requests benchmark on a single channel, 10 consumers")
	logging.ConfigureDefaultLogger("system-default", "warn", false)

	nWarmupRequests := 10_000
	nRequests := 1_000_000
	nGoRoutines := 100

	ch := make(chan struct{}, 1_000)

	runBenchmark := func(nReq int) {
		go func() {
			lop.ForEach(lo.Range(nGoRoutines), func(iact int, _ int) {
				for i := 0; i < nReq/nGoRoutines; i++ {
					ch <- struct{}{}
				}
			})
		}()

		lop.ForEach(lo.Range(10), func(iact int, _ int) {
			for i := 0; i < nReq/10; i++ {
				<-ch
			}
		})
	}

	// warmup
	runBenchmark(nWarmupRequests)

	// actual benchmark
	t0 := time.Now()
	runBenchmark(nRequests)

	elapsed := time.Since(t0)

	msgRate := float64(nRequests) / elapsed.Seconds()

	logging.ConfigureDefaultLogger("system-default", "info", false)
	slog.Info("Benchmark passed")
	slog.Info(fmt.Sprintf("Msg rate: %s msg/s", fmtLargeIntIn3DigitSets(int64(msgRate))))
	slog.Info("Benchmark took", slog.Duration("duration", elapsed))

}

func run1mChannelRequests10Chan10Cons() {

	slog.Info("********************************************************************")
	slog.Info("* Running 1m requests benchmark on 10 channels, 10 consumers")
	logging.ConfigureDefaultLogger("system-default", "warn", false)

	nWarmupRequests := 10_000
	nRequests := 1_000_000
	nGoRoutines := 100

	chs := make([]chan struct{}, 10)
	for i := range chs {
		chs[i] = make(chan struct{}, 1_000)
	}

	runBenchmark := func(nReq int) {
		go func() {
			lop.ForEach(lo.Range(nGoRoutines), func(iCh int, _ int) {
				ch := chs[iCh%10]
				for i := 0; i < nReq/nGoRoutines; i++ {
					ch <- struct{}{}
				}
			})
		}()

		lop.ForEach(lo.Range(10), func(iCh int, _ int) {
			ch := chs[iCh]
			for i := 0; i < nReq/10; i++ {
				<-ch
			}
		})
	}

	// warmup
	runBenchmark(nWarmupRequests)

	// actual benchmark
	t0 := time.Now()
	runBenchmark(nRequests)

	elapsed := time.Since(t0)

	msgRate := float64(nRequests) / elapsed.Seconds()

	logging.ConfigureDefaultLogger("system-default", "info", false)
	slog.Info("Benchmark passed")
	slog.Info(fmt.Sprintf("Msg rate: %s msg/s", fmtLargeIntIn3DigitSets(int64(msgRate))))
	slog.Info("Benchmark took", slog.Duration("duration", elapsed))

}

func run1mChannelRequests100Chan100Cons() {

	slog.Info("********************************************************************")
	slog.Info("* Running 1m requests benchmark on 100 channels, 100 consumers")
	logging.ConfigureDefaultLogger("system-default", "warn", false)

	nWarmupRequests := 10_000
	nRequests := 1_000_000
	nGoRoutines := 100
	nChans := 100

	chs := make([]chan struct{}, nChans)
	for i := range chs {
		chs[i] = make(chan struct{}, 1_000)
	}

	runBenchmark := func(nReq int) {
		go func() {
			lop.ForEach(lo.Range(nGoRoutines), func(iCh int, _ int) {
				ch := chs[iCh%nChans]
				for i := 0; i < nReq/nGoRoutines; i++ {
					ch <- struct{}{}
				}
			})
		}()

		lop.ForEach(lo.Range(nChans), func(iCh int, _ int) {
			ch := chs[iCh]
			for i := 0; i < nReq/nChans; i++ {
				<-ch
			}
		})
	}

	// warmup
	runBenchmark(nWarmupRequests)

	// actual benchmark
	t0 := time.Now()
	runBenchmark(nRequests)

	elapsed := time.Since(t0)

	msgRate := float64(nRequests) / elapsed.Seconds()

	logging.ConfigureDefaultLogger("system-default", "info", false)
	slog.Info("Benchmark passed")
	slog.Info(fmt.Sprintf("Msg rate: %s msg/s", fmtLargeIntIn3DigitSets(int64(msgRate))))
	slog.Info("Benchmark took", slog.Duration("duration", elapsed))

}

func run1mChannelRequests1000Chan1000Cons() {

	slog.Info("********************************************************************")
	slog.Info("* Running 1m requests benchmark on 1000 channels, 1000 consumers")
	logging.ConfigureDefaultLogger("system-default", "warn", false)

	nWarmupRequests := 10_000
	nRequests := 1_000_000
	nGoRoutines := 1000
	nChans := 1000

	chs := make([]chan struct{}, nChans)
	for i := range chs {
		chs[i] = make(chan struct{}, 1_000)
	}

	runBenchmark := func(nReq int) {
		go func() {
			lop.ForEach(lo.Range(nGoRoutines), func(iCh int, _ int) {
				ch := chs[iCh%nChans]
				for i := 0; i < nReq/nGoRoutines; i++ {
					ch <- struct{}{}
				}
			})
		}()

		lop.ForEach(lo.Range(nChans), func(iCh int, _ int) {
			ch := chs[iCh]
			for i := 0; i < nReq/nChans; i++ {
				<-ch
			}
		})
	}

	// warmup
	runBenchmark(nWarmupRequests)

	// actual benchmark
	t0 := time.Now()
	runBenchmark(nRequests)

	elapsed := time.Since(t0)

	msgRate := float64(nRequests) / elapsed.Seconds()

	logging.ConfigureDefaultLogger("system-default", "info", false)
	slog.Info("Benchmark passed")
	slog.Info(fmt.Sprintf("Msg rate: %s msg/s", fmtLargeIntIn3DigitSets(int64(msgRate))))
	slog.Info("Benchmark took", slog.Duration("duration", elapsed))

}

func run1mChannelRequests10000Chan10000Cons() {

	slog.Info("********************************************************************")
	slog.Info("* Running 1m requests benchmark on 10_000 channels, 10_000 consumers")
	logging.ConfigureDefaultLogger("system-default", "warn", false)

	nWarmupRequests := 10_000
	nRequests := 1_000_000
	nGoRoutines := 10_000
	nChans := 10_000

	chs := make([]chan struct{}, nChans)
	for i := range chs {
		chs[i] = make(chan struct{}, 1_000)
	}

	runBenchmark := func(nReq int) {
		go func() {
			lop.ForEach(lo.Range(nGoRoutines), func(iCh int, _ int) {
				ch := chs[iCh%nChans]
				for i := 0; i < nReq/nGoRoutines; i++ {
					ch <- struct{}{}
				}
			})
		}()

		lop.ForEach(lo.Range(nChans), func(iCh int, _ int) {
			ch := chs[iCh]
			for i := 0; i < nReq/nChans; i++ {
				<-ch
			}
		})
	}

	// warmup
	runBenchmark(nWarmupRequests)

	// actual benchmark
	t0 := time.Now()
	runBenchmark(nRequests)

	elapsed := time.Since(t0)

	msgRate := float64(nRequests) / elapsed.Seconds()

	logging.ConfigureDefaultLogger("system-default", "info", false)
	slog.Info("Benchmark passed")
	slog.Info(fmt.Sprintf("Msg rate: %s msg/s", fmtLargeIntIn3DigitSets(int64(msgRate))))
	slog.Info("Benchmark took", slog.Duration("duration", elapsed))

}

func fmtLargeIntIn3DigitSets(n int64) string {
	s := fmt.Sprintf("%d", n)
	if len(s) < 4 {
		return s
	}
	return fmtLargeIntIn3DigitSets(n/1_000) + "," + s[len(s)-3:]
}
