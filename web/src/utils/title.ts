// Accessible page titles: "<page> · Pawn.Ink".
let appName = 'Pawn.Ink';

export function setAppName(name: string): void {
  appName = name;
}

export function setTitle(page?: string): void {
  document.title = page ? `${page} · ${appName}` : appName;
}
