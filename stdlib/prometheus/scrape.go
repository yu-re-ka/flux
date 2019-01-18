package inputs

import (
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"

	"time"

	"github.com/influxdata/flux"
	"github.com/influxdata/flux/execute"
	"github.com/influxdata/flux/plan"
	"github.com/influxdata/flux/semantic"
	"github.com/influxdata/flux/stdlib/inputs"
	"github.com/prometheus/common/expfmt"
)

const (
	// DefaultTimeout is the length of time requesting metrics from a target
	// before canceling.
	DefaultTimeout = 10 * time.Second
	// DefaultMaxBytes is the largest request body read (10MB).
	DefaultMaxBytes = 10000000
	// just in case the definition of time.Nanosecond changes from 1.
	nsPerMilliseconds = int64(time.Millisecond / time.Nanosecond)
)

// ScrapePrometheusKind is the identity of this flux operation.
const ScrapePrometheusKind = "scraperPrometheus"

// ScrapePrometheusOpSpec define the required information to the operation.
type ScrapePrometheusOpSpec struct {
	Target  string        `json:"target,omitempty"`
	Timeout time.Duration `json:"timeout,omitempty"`
}

func init() {
	scrapePrometheusSignature := semantic.FunctionPolySignature{
		Parameters: map[string]semantic.PolyType{
			"target":  semantic.String,
			"timeout": semantic.Duration,
		},
		Required: semantic.LabelSet{"target"},
		Return:   flux.TableObjectType,
	}

	flux.RegisterPackageValue("prometheus",
		ScrapePrometheusKind,
		flux.FunctionValue(
			ScrapePrometheusKind,
			createScrapePrometheusOpSpec,
			scrapePrometheusSignature,
		),
	)

	flux.RegisterOpSpec(ScrapePrometheusKind, newScrapePrometheusOp)

	plan.RegisterProcedureSpec(
		ScrapePrometheusKind,
		newScrapePrometheusProcedure,
		ScrapePrometheusKind,
	)

	execute.RegisterSource(ScrapePrometheusKind, createScrapePrometheusSource)
}

func createScrapePrometheusOpSpec(args flux.Arguments, administration *flux.Administration) (spec flux.OperationSpec, err error) {
	spec = new(ScrapePrometheusOpSpec)
	spec.Target, err = args.GetRequiredString("target")
	if err != nil {
		return nil, err
	}

	spec.Timeout, ok, err = args.GetDuration("timeout")
	if err != nil {
		return err
	}

	if !ok {
		spec.Timeout = DefaultTimeout
	}

	return spec, nil
}

func newScrapePrometheusOp() flux.OperationSpec {
	return new(ScrapePrometheusOpSpec)
}

// Kind returns the unique kind of this operation specification.
func (s *ScrapePrometheusOpSpec) Kind() flux.OperationKind {
	return ScrapePrometheusKind
}

// ScrapePrometheusProcedureSpec defines the scrape procedure.
type ScrapePrometheusProcedureSpec struct {
	plan.DefaultCost

	Target  string
	Timeout time.Duration
}

func newScrapePrometheusProcedure(qs flux.OperationSpec, pa plan.Administration) (plan.ProcedureSpec, error) {
	spec, ok := qs.(*ScrapePrometheusOpSpec)
	if !ok {
		return nil, fmt.Errorf("invalid spec type %T", qs)
	}

	return &ScrapePrometheusProcedureSpec{
		Target:  spec.Target,
		Timeout: spec.Timeout,
	}, nil
}

// Kind returns the unique name of this procedure spec.
func (s *ScrapePrometheusProcedureSpec) Kind() plan.ProcedureKind {
	return ScrapePrometheusKind
}

// Copy is a shallow copy of this procedure spec.
func (s *ScrapePrometheusProcedureSpec) Copy() plan.ProcedureSpec {
	ns := new(ScrapePrometheusProcedureSpec)
	ns.Target = s.Target
	return ns
}

func createScrapePrometheusSource(prSpec plan.ProcedureSpec, dsid execute.DatasetID, a execute.Administration) (execute.Source, error) {
	spec, ok := prSpec.(*ScrapePrometheusProcedureSpec)
	if !ok {
		return nil, fmt.Errorf("invalid spec type %T", prSpec)
	}

	ScrapeDecoder := ScrapeDecoder{
		Target:         spec.Target,
		id:             dsid,
		spec:           spec,
		administration: a,
		timeout:        spec.Timeout,
	}

	return inputs.CreateSourceFromDecoder(&ScrapeDecoder, dsid, a)
}

// ScrapeDecoder connects to the target, downloads prom metrics, and converts
// them to flux.Tables.
type ScrapeDecoder struct {
	Target  string
	timeout time.Duration

	id             execute.DatasetID
	administration execute.Administration
	spec           *ScrapePrometheusProcedureSpec

	dec        Scraper
	mfs        []*dto.MetricFamily
	fetched    bool
	moreTables bool
	tables     []flux.Table
	tableIdx   int
}

// Connect checks if the Target URL is formatted correct and
// creates an HTTP request.
func (c *ScrapeDecoder) Connect() error {
	u, err := url.Parse(Target)
	if err != nil {
		return err
	}

	c.dec, err = NewScraper(u, c.timeout)
	return err
}

// Fetch connects to scraper target and retrieves the metrics.
func (c *ScrapeDecoder) Fetch() (bool, error) {
	if !c.fetched {
		mfs, err := c.dec.Scrape(context.Background())
		if err != nil {
			return false, err
		}

		c.fetched = true
		c.mfs = mfs
		c.moreTables = len(mfs) > 1
	}

	return c.moreTables, nil
}

func (c *ScrapeDecoder) newTables(mfs []*dto.MetricFamily) []flux.Table {
	tables := make([]flux.Table{}, 0, len(mfs))
	for _, mf := range mfs {
		// what should the group key be?
		groupKey := execute.NewGroupKey(nil, nil)
		builder := execute.NewColListTableBuilder(
			groupKey,
			c.administration.Allocator(),
		)

		name := value.NewString(mf.GetName())
		measurement := flux.ColMeta{
			Label: "_measurement",
			Type:  flux.TString,
		}

		tm := flux.ColMeta{
			Label: "_time",
			Type:  flux.TTime,
		}

		for _, metric := range mf.Metric {
			// Add all the values first as that is
			// the total number of rows.
			vs := fieldValues(mf.GetType(), metric)
			for _, v := range vs {
				n, err := builder.AddCol(flux.ColMeta{
					Label: "_field",
					Type:  flux.TString,
				})

				if err != nil {
					return nil, err
				}

				if err := builder.AppendValue(n, v[0]); err != nil {
					return nil, err
				}

				m, err := builder.AddCol(flux.ColMeta{
					Label: "_value",
					Type:  flux.TString,
				})
				if err != nil {
					return nil, err
				}

				if err := builder.AppendValue(m, v[1]); err != nil {
					return nil, err
				}

			}

			// Write the name, labels, and timestamp into the same column for each row.
			n, err := builder.AddCol(measurement)
			if err != nil {
				return nil, err
			}

			for i := 0; i < builder.NRows(); i++ {
				if err := builder.AppendValue(n, name); err != nil {
					return nil, err
				}
			}

			n, err := builder.AddCol(tm)
			if err != nil {
				return nil, err
			}

			t := now
			if m.GetTimestampMs() > 0 {
				t = time.Unix(0, m.GetTimestampMs()*nsPerMilliseconds)
			}

			for i := 0; i < builder.NRows(); i++ {
				if err := builder.AppendValue(n, value.NewTime(t)); err != nil {
					return nil, err
				}
			}

			for _, lp := range m.Label {
				c := flux.ColMeta{
					Label: lp.GetName(),
					Type:  flux.TString,
				}
				n, err := builder.AddCol(c)
				if err != nil {
					return nil, err
				}

				labelValue := value.NewString(lp.GetValue())
				for i := 0; i < builder.NRows(); i++ {
					if err := builder.AppendValue(n, labelValue); err != nil {
						return nil, err
					}
				}
			}
		}
		tables = append(builder.Table())
	}
	return tables, nil
}

// Decode converts prometheus metrics families to flux.Tables.
func (c *ScrapeDecoder) Decode() (flux.Table, error) {
	if c.tables != nil {
		tables, err := newTables(c.mfs)
		if err != nil {
			return nil, error
		}
		c.tables = tables
	}

	var table flux.Table
	if c.tableIdx < len(c.tables) {
		table = c.tables[tableIdx]
		c.tableIdx++

		if c.tableIdx == len(c.tables) {
			c.moreTables = false
		}

	}

	return table, nil
}

// Scraper requests data from a scrape target and returns prometheus
// parsed prometheus model data.
type Scraper struct {
	URL     url.URL
	Timeout time.Duration

	req *http.Request
}

// NewScraper creates a new scraper and validates the HTTP request.
func NewScraper(u url.URL, timeout time.Duration) (*Scraper, error) {
	s := &Scraper{
		URL:     u,
		Timeout: timeout,
	}

	var err error
	s.req, err = s.newRequest()
	return s, err
}

const (
	acceptHeader    = `text/plain;version=0.0.4; charset=utf-8`
	userAgentHeader = `Flux/0.1`
)

func (s *Scaper) newRequest() (*http.Request, error) {
	req, err := http.NewRequest("GET", s.URL.String(), nil)
	if err != nil {
		return nil, err
	}

	req.Header.Add("Accept", acceptHeader)
	req.Header.Add("Accept-Encoding", "gzip")
	req.Header.Set("User-Agent", userAgentHeader)
	req.Header.Set("X-Prometheus-Scrape-Timeout-Seconds", fmt.Sprintf("%f", s.timeout.Seconds()))

	return req, nil
}

// Scrape contacts the scrape target and downloads the prometheus metrics.
func (s *Scraper) Scrape(ctx context.Context) ([]*dto.MetricFamily, error) {
	ctx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	req, err := http.NewRequest("GET", s.URL.String(), nil)
	if err != nil {
		return nil, err
	}
	if s.req == nil {
		s.req, err = s.newRequest()
		if err != nil {
			return nil, err
		}
	}

	resp, err := http.Do(s.req.WithContext(ctx))
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unable to retrieve metrics; received HTTP status %s", resp.Status)
	}

	return decodeMetricsResponse(resp, DefaultMaxBytes)
}

func decodeMetricsResponse(resp *http.Response, maxBytes int64) ([]*dto.MetricFamily, error) {
	format := expfmt.ResponseFormat(resp.Header)
	if format == expfmt.FmtUnknown {
		return nil, fmt.Errorf("unknown format metrics format")
	}

	// protect against reading too many bytes
	r := io.LimitReader(resp.Body, maxBytes)
	defer resp.Body.Close()

	if resp.Header.Get("Content-Encoding") == "gzip" {
		gr, err := gzip.NewReader(s.buf)
		if err != nil {
			return nil, err
		}
		defer gr.Close()
	}

	mfs, err := decodeExpfmt(r, format)
	if err != nil {
		return nil, err
	}
	return mfs, nil
}

func decodeExpfmt(r io.Reader, format expfmt.Format) ([]*dto.MetricFamily, error) {
	dec := expfmt.NewDecoder(r, format)
	mfs := []*dto.MetricFamily{}
	for {
		var mf dto.MetricFamily
		if err := dec.Decode(&mf); err != nil {
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, err
			}
		}
		mfs = append(mfs, &mf)
	}
	return mfs, nil
}

func summary(s *dto.Summary) [][]value.Value {
	vs := make([][]value.Value, 0, 2+len(s.Quantile))
	vs = append(vs,
		[]value.Value{
			value.NewString("count"),
			value.NewFloat(float64(s.GetSampleCount())),
		},
		[]value.Value{
			value.NewString("sum"),
			value.NewFloat(float64(s.GetSampleSum())),
		},
	)

	for _, q := range s.Quantile {
		if !math.IsNaN(q.GetValue()) {
			v := []value.Value{
				value.NewString(fmt.Sprint(q.GetQuantile())),
				value.NewFloat(q.GetValue()),
			}
			vs = append(vs, v)
		}

	}
	return vs
}

func histogram(h *dto.Histogram) [][]value.Value {
	vs := make([][]value.Value, 0, 2+len(h.Bucket))
	vs = append(vs,
		[]value.Value{
			value.NewString("count"),
			value.NewFloat(float64(h.GetSampleCount())),
		},
		[]value.Value{
			value.NewString("sum"),
			value.NewFloat(float64(h.GetSampleSum())),
		},
	)

	for _, b := range h.Bucket {
		v := []value.Value{
			value.NewString(fmt.Sprint(b.GetUpperBound())),
			value.NewFloat(float64(b.GetCumulativeCount())),
		}
		vs = append(vs, v)
	}

	return vs
}

type valuer interface {
	GetValue() float64
}

func valuer(typ string, m valuer) [][]value.Value {
	vs := make([][]value.Value, 0, 1)

	v := m.GetValue()
	if !math.IsNaN(v) {
		vs = append(vs, []value.Value{
			value.NewString(typ),
			value.NewFloat(v),
		})
	}

	return vs
}

// returns a slice of fieldValue pairs
func fieldValues(typ dto.MetricType, m *dto.Metric) [][]value.Value {
	switch typ {
	case dto.MetricType_SUMMARY:
		return summary(m.GetSummary())
	case dto.MetricType_HISTOGRAM:
		return histogram(m.GetHistogram())
	case dto.MetricType_GAUGE:
		return valuer("gauge", m.GetGauge())
	case dto.MetricType_COUNTER:
		return valuer("counter", m.GetCounter())
	case dto.MetricType_UNTYPED:
		return valuer("value", m.GetUntyped())
	default:
		return nil
	}
}
