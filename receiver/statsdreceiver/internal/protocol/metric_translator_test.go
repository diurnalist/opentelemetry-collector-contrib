// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package protocol

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/pdata/pcommon"
	"go.opentelemetry.io/collector/pdata/pmetric"
	"go.opentelemetry.io/otel/attribute"
)

func TestBuildCounterMetric(t *testing.T) {
	metricDescription := statsDMetricDescription{
		name:  "testCounter",
		attrs: attribute.NewSet(attribute.String("mykey", "myvalue")),
	}

	parsedMetric := statsDMetric{
		description: metricDescription,
		asFloat:     32,
		unit:        "meter",
	}
	isMonotonicCounter := false
	metric := buildCounterMetric(parsedMetric, isMonotonicCounter)
	expectedMetrics := pmetric.NewScopeMetrics()
	expectedMetric := expectedMetrics.Metrics().AppendEmpty()
	expectedMetric.SetName("testCounter")
	expectedMetric.SetUnit("meter")
	expectedMetric.SetEmptySum().SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
	expectedMetric.Sum().SetIsMonotonic(isMonotonicCounter)
	dp := expectedMetric.Sum().DataPoints().AppendEmpty()
	dp.SetIntValue(32)
	dp.Attributes().PutStr("mykey", "myvalue")
	assert.Equal(t, metric, expectedMetrics)
}

func TestSetTimestampsForCounterMetric(t *testing.T) {
	timeNow := time.Now()
	lastUpdateInterval := timeNow.Add(-1 * time.Minute)

	parsedMetric := statsDMetric{}
	isMonotonicCounter := false
	metric := buildCounterMetric(parsedMetric, isMonotonicCounter)
	setTimestampsForCounterMetric(metric, lastUpdateInterval, timeNow)

	expectedMetrics := pmetric.NewScopeMetrics()
	expectedMetric := expectedMetrics.Metrics().AppendEmpty()
	expectedMetric.SetEmptySum().SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
	dp := expectedMetric.Sum().DataPoints().AppendEmpty()
	dp.SetStartTimestamp(pcommon.NewTimestampFromTime(lastUpdateInterval))
	dp.SetTimestamp(pcommon.NewTimestampFromTime(timeNow))
	assert.Equal(t,
		metric.Metrics().At(0).Sum().DataPoints().At(0).StartTimestamp(),
		expectedMetrics.Metrics().At(0).Sum().DataPoints().At(0).StartTimestamp(),
	)
	assert.Equal(t,
		metric.Metrics().At(0).Sum().DataPoints().At(0).Timestamp(),
		expectedMetrics.Metrics().At(0).Sum().DataPoints().At(0).Timestamp(),
	)

}

func TestBuildGaugeMetric(t *testing.T) {
	timeNow := time.Now()
	metricDescription := statsDMetricDescription{
		name: "testGauge",
		attrs: attribute.NewSet(
			attribute.String("mykey", "myvalue"),
			attribute.String("mykey2", "myvalue2"),
		),
	}
	parsedMetric := statsDMetric{
		description: metricDescription,
		asFloat:     32.3,
		unit:        "meter",
	}
	metric := buildGaugeMetric(parsedMetric, timeNow)
	expectedMetrics := pmetric.NewScopeMetrics()
	expectedMetric := expectedMetrics.Metrics().AppendEmpty()
	expectedMetric.SetName("testGauge")
	expectedMetric.SetUnit("meter")
	dp := expectedMetric.SetEmptyGauge().DataPoints().AppendEmpty()
	dp.SetDoubleValue(32.3)
	dp.SetTimestamp(pcommon.NewTimestampFromTime(timeNow))
	dp.Attributes().PutStr("mykey", "myvalue")
	dp.Attributes().PutStr("mykey2", "myvalue2")
	assert.Equal(t, metric, expectedMetrics)
}

func TestBuildSummaryMetricUnsampled(t *testing.T) {
	timeNow := time.Now()

	unsampledMetric := summaryMetric{
		points:  []float64{1, 2, 4, 6, 5, 3},
		weights: []float64{1, 1, 1, 1, 1, 1},
	}

	attrs := attribute.NewSet(
		attribute.String("mykey", "myvalue"),
		attribute.String("mykey2", "myvalue2"),
	)

	desc := statsDMetricDescription{
		name:       "testSummary",
		metricType: HistogramType,
		attrs:      attrs,
	}

	metric := pmetric.NewScopeMetrics()
	buildSummaryMetric(desc, unsampledMetric, timeNow.Add(-time.Minute), timeNow, statsDDefaultPercentiles, metric)

	expectedMetric := pmetric.NewScopeMetrics()
	m := expectedMetric.Metrics().AppendEmpty()
	m.SetName("testSummary")
	dp := m.SetEmptySummary().DataPoints().AppendEmpty()
	dp.SetSum(21)
	dp.SetCount(6)
	dp.SetStartTimestamp(pcommon.NewTimestampFromTime(timeNow.Add(-time.Minute)))
	dp.SetTimestamp(pcommon.NewTimestampFromTime(timeNow))
	for _, kv := range desc.attrs.ToSlice() {
		dp.Attributes().PutStr(string(kv.Key), kv.Value.AsString())
	}
	quantile := []float64{0, 10, 50, 90, 95, 100}
	value := []float64{1, 1, 3, 6, 6, 6}
	for int, v := range quantile {
		eachQuantile := dp.QuantileValues().AppendEmpty()
		eachQuantile.SetQuantile(v / 100)
		eachQuantileValue := value[int]
		eachQuantile.SetValue(eachQuantileValue)
	}

	assert.Equal(t, expectedMetric, metric)
}

func TestBuildSummaryMetricSampled(t *testing.T) {
	timeNow := time.Now()

	type testCase struct {
		points      []float64
		weights     []float64
		count       uint64
		sum         float64
		percentiles []float64
		values      []float64
	}

	for _, test := range []testCase{
		{
			points:      []float64{1, 2, 3},
			weights:     []float64{100, 1, 100},
			count:       201,
			sum:         402,
			percentiles: []float64{0, 1, 49, 50, 51, 99, 100},
			values:      []float64{1, 1, 1, 2, 3, 3, 3},
		},
		{
			points:      []float64{1, 2},
			weights:     []float64{99, 1},
			count:       100,
			sum:         101,
			percentiles: []float64{0, 98, 99, 100},
			values:      []float64{1, 1, 1, 2},
		},
		{
			points:      []float64{0, 1, 2, 3, 4, 5},
			weights:     []float64{1, 9, 40, 40, 5, 5},
			count:       100,
			sum:         254,
			percentiles: statsDDefaultPercentiles,
			values:      []float64{0, 1, 2, 3, 4, 5},
		},
	} {
		sampledMetric := summaryMetric{
			points:  test.points,
			weights: test.weights,
		}

		attrs := attribute.NewSet(
			attribute.String("mykey", "myvalue"),
			attribute.String("mykey2", "myvalue2"),
		)

		desc := statsDMetricDescription{
			name:       "testSummary",
			metricType: HistogramType,
			attrs:      attrs,
		}

		metric := pmetric.NewScopeMetrics()
		buildSummaryMetric(desc, sampledMetric, timeNow.Add(-time.Minute), timeNow, test.percentiles, metric)

		expectedMetric := pmetric.NewScopeMetrics()
		m := expectedMetric.Metrics().AppendEmpty()
		m.SetName("testSummary")
		dp := m.SetEmptySummary().DataPoints().AppendEmpty()

		dp.SetSum(test.sum)
		dp.SetCount(test.count)

		dp.SetStartTimestamp(pcommon.NewTimestampFromTime(timeNow.Add(-time.Minute)))
		dp.SetTimestamp(pcommon.NewTimestampFromTime(timeNow))
		for _, kv := range desc.attrs.ToSlice() {
			dp.Attributes().PutStr(string(kv.Key), kv.Value.AsString())
		}
		for i := range test.percentiles {
			eachQuantile := dp.QuantileValues().AppendEmpty()
			eachQuantile.SetQuantile(test.percentiles[i] / 100)
			eachQuantile.SetValue(test.values[i])
		}

		assert.Equal(t, expectedMetric, metric)
	}
}

func TestBuildHistogramMetric(t *testing.T) {
	timeNow := time.Now()
	metricDescription := statsDMetricDescription{
		name: "testHistogram",
		attrs: attribute.NewSet(
			attribute.String("mykey", "myvalue"),
			attribute.String("mykey2", "myvalue2"),
		),
	}

	parsedMetric := explicitHistogramMetric{
		points: []float64{
			0,
			0.0001, // 0.1ms
			0.0010, // 1ms
			0.0100, // 10ms
			0.0250, // 25ms
			0.0500, // 50ms
			0.1000, // 100ms
			1.0,    // 1s
			10.0,   // 10s
		},
	}
	metrics := pmetric.NewScopeMetrics()
	timeStart := timeNow.Add(-time.Minute)
	buildHistogramMetric(metricDescription, parsedMetric, timeStart, timeNow, metrics)

	expectedMetrics := pmetric.NewScopeMetrics()
	expectedMetric := expectedMetrics.Metrics().AppendEmpty()
	expectedMetric.SetName("testHistogram")
	expectedMetric.SetEmptyHistogram().SetAggregationTemporality(pmetric.AggregationTemporalityDelta)
	dp := expectedMetric.Histogram().DataPoints().AppendEmpty()
	dp.SetTimestamp(pcommon.NewTimestampFromTime(timeNow))
	dp.SetStartTimestamp(pcommon.NewTimestampFromTime(timeStart))
	dp.SetCount(9)
	dp.SetMin(0)
	dp.SetMax(10)
	dp.SetSum(11.1861)
	dp.BucketCounts().FromRaw([]uint64{
		1, // 0
		0, // 2^-14
		1, // 2^-13
		0, // 2^-12
		0, // 2^-11
		0, // 2^-10
		1, // 2^-9
		0, // 2^-8
		0, // 2^-7
		1, // 2^-6
		1, // 2^-5
		1, // 2^-4
		1, // 2^-3
		0, // 2^-2
		0, // 2^-1
		1, // 2^0
		0, // 2^1
		0, // 2^2
		0, // 2^3
		1, // 2^4
		0, // 2^5
		0, // 2^6
		0, // 2^7
		0, // 2^8
		0, // 2^9
		0, // 2^10
		0, // 2^11
		0, // 2^12
		0, // 2^13
		0, // 2^14
		0, // 2^15
	})
	dp.ExplicitBounds().FromRaw(explicitHistogramBoundaries)
	dp.Attributes().PutStr("mykey", "myvalue")
	dp.Attributes().PutStr("mykey2", "myvalue2")

	assert.Equal(t, metrics, expectedMetrics)
}
