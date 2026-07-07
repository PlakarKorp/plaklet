package plaklet

// Built-in connectors, registered in-process at init time via their package
// side effects. This is the same set the public plakar CLI compiles in. Unlike
// the plakman executor, plaklet does not load connectors as out-of-process gRPC
// plugins here: the backends it supports are the ones linked below.
//
// To support more sources/destinations/stores, add the corresponding
// integration subpackage import.
import (
	_ "github.com/PlakarKorp/integrations/fs/exporter"
	_ "github.com/PlakarKorp/integrations/fs/importer"
	_ "github.com/PlakarKorp/integrations/fs/storage"
	_ "github.com/PlakarKorp/integrations/http/storage"
	_ "github.com/PlakarKorp/integrations/ptar/storage"
	_ "github.com/PlakarKorp/integrations/stdio/exporter"
	_ "github.com/PlakarKorp/integrations/stdio/importer"
	_ "github.com/PlakarKorp/integrations/tar/importer"
)
