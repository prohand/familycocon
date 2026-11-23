// SPDX-License-Identifier: GPL-3.0-or-later

package base

import (
	"database/sql/driver"

	"modernc.org/sqlite"

	"familycocon/internal/texte"
)

// Fonction SQL « plier(x) » : le texte en minuscules et sans accents
// (texte.Plier). lower() de SQLite ne change que les lettres sans accent :
// « ÉCOLE » ne trouvait pas « école », ni « noel » « Noël ».
func init() {
	err := sqlite.RegisterDeterministicScalarFunction("plier", 1, func(_ *sqlite.FunctionContext, args []driver.Value) (driver.Value, error) {
		switch v := args[0].(type) {
		case string:
			return texte.Plier(v), nil
		case []byte:
			return texte.Plier(string(v)), nil
		case nil:
			return nil, nil
		default:
			return v, nil
		}
	})
	if err != nil {
		panic(err)
	}
}
