package operator

import (
	"context"
	"crypto/tls"
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	ctrlruntimemetrics "sigs.k8s.io/controller-runtime/pkg/metrics"
)

// StartMetricsListener starts the metrics listener on addr.  When certFile
// and keyFile are non-empty the server uses TLS with the given tlsConfig;
// otherwise it falls back to plain HTTP.
func StartMetricsListener(addr, certFile, keyFile string, tlsConfig *tls.Config, signal context.Context) {
	// These metrics get registered in controller-runtime's registry via an init in the internal/controller/metrics package.
	// Unregister the controller-runtime metrics, so that we can combine the controller-runtime metric's registry
	// with that of the ingress-operator. This shouldn't have any side effects, as long as no 2 metrics across
	// controller runtime or the ingress operator share the same name (which is unlikely). See
	// https://github.com/kubernetes/test-infra/blob/master/prow/metrics/metrics.go for additional context.
	ctrlruntimemetrics.Registry.Unregister(collectors.NewGoCollector(collectors.WithGoCollectorRuntimeMetrics(collectors.MetricsAll)))
	ctrlruntimemetrics.Registry.Unregister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

	// Create prometheus handler by combining the ingress-operator registry
	// with the ingress-operator's controller runtime metrics registry.
	handler := promhttp.HandlerFor(
		prometheus.Gatherers{prometheus.DefaultGatherer, ctrlruntimemetrics.Registry},
		promhttp.HandlerOpts{},
	)

	mux := http.NewServeMux()
	mux.Handle("/metrics", handler)
	s := http.Server{Addr: addr, Handler: mux}

	useTLS := certFile != "" && keyFile != ""
	if useTLS {
		s.TLSConfig = tlsConfig
		log.Info("starting metrics listener with TLS", "addr", addr)
	} else {
		log.Info("starting metrics listener", "addr", addr)
	}

	go func() {
		var err error
		if useTLS {
			err = s.ListenAndServeTLS(certFile, keyFile)
		} else {
			err = s.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			log.Error(err, "metrics listener exited")
		}
	}()
	<-signal.Done()
	if err := s.Shutdown(context.Background()); err != http.ErrServerClosed {
		log.Error(err, "error stopping metrics listener")
	}
}
