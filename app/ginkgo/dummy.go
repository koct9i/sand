package ginkgo

import (
	"context"
	"math/rand"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/onsi/gomega/gmeasure"
)

var _ = Context("Dummy", Label("dummy"), func() {
	logger := GinkgoLogr

	It("Always succeed", Label("ok"), func() {
		logger.Info("Hello")
		Expect(1 + 1).To(Equal(2))
	})

	It("Always fails", Label("fail"), func() {
		logger.Info("Hello")
		Fail("as expected")
	})

	It("Fails randomly", Label("random"), func() {
		if rand.Intn(6) == 0 {
			Fail("Bang!")
		}
	})

	It("Fails randomly but deterministic depending on seed", Label("random", "deterministic"), func() {
		if NewSeededRand().Intn(6) == 0 {
			Fail("Bang!")
		}
	})

	It("Always skipped", Label("skip"), func() {
		Skip("As usual")
	})

	It("Always stuck", Label("timeout"), func(ctx context.Context) {
		Skip("Not now")
		logger.Info("Hello")
		<-ctx.Done()
	})

	It("Measure randomness", Label("measure"), func() {
		exp := NewExperiment("d6")
		dice := NewSeededRand()
		exp.SampleValue("roll", func(idx int) float64 {
			return float64(dice.Intn(6) + 1)
		}, SamplingConfig{N: 100, Duration: time.Second})
		AddReportEntry(exp.Name, exp)
	})


	It("Measure rps", Label("rps"), func() {
		experiment := NewExperiment("rps")
		dice := NewSeededRand()
		experiment.SampleDuration("dice", func(idx int) {
			time.Sleep(time.Millisecond*time.Duration(dice.Intn(6)+1))
		}, SamplingConfig{Duration: time.Second*5})
		AddReportEntry(experiment.Name, experiment)
	})
})
