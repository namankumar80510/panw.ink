// Parcel empties dist/ on build; the Go //go:embed directive needs the
// tracked dist/.gitkeep to exist on a fresh clone, so restore it.
import { mkdirSync, writeFileSync } from 'node:fs';
mkdirSync('dist', { recursive: true });
writeFileSync('dist/.gitkeep', '');
