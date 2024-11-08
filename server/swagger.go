package server

import (
	"fmt"
	"net/http"

	"github.com/ctfer-io/chall-manager/global"
	"github.com/ctfer-io/chall-manager/pkg/swagger"
	swaggerui "github.com/ctfer-io/chall-manager/swagger-ui"
)

func addSwagger(mux *http.ServeMux) {
	mux.HandleFunc("/swagger/swagger.json", func(w http.ResponseWriter, r *http.Request) {
		_, span := global.Tracer.Start(r.Context(), "swagger")
		defer span.End()

		swaggers := []string{
			"challenge",
			"instance",
			"common", // must be last to overwrite previous attributes
		}
		mergedSwagger := swagger.NewMerger()
		for _, swagger := range swaggers {
			swaggerPath := fmt.Sprintf("./gen/api/v1/%[1]s/%[1]s.swagger.json", swagger)
			if err := mergedSwagger.AddFile(swaggerPath); err != nil {
				http.Error(w, "Merging swaggers", http.StatusInternalServerError)
				return
			}
		}
		b, err := mergedSwagger.MarshalJSON()
		if err != nil {
			http.Error(w, "Exporting merged swagger", http.StatusInternalServerError)
			return
		}
		if _, err := w.Write(b); err != nil {
			http.Error(w, "Writing  merged swagger", http.StatusInternalServerError)
			return
		}
	})
	mux.Handle("/swagger/", http.StripPrefix("/swagger/", http.FileServer(http.FS(swaggerui.Content))))
}
