import { reader } from '../api/reader';
import { setAppName } from '../utils/title';

export interface PublicConfig {
  app_name: string;
  base_url: string;
  environment: string;
  non_production: boolean;
  features: { registration: boolean; early_access: boolean; analytics: boolean; test_entitlements: boolean };
  limits: { password_min: number; password_max: number; max_early_days: number; page_size_max: number };
  patreon_url: string | null;
  support_contact: string | null;
}

export const config = {
  value: null as PublicConfig | null,
  async load(): Promise<void> {
    try {
      this.value = await reader.get<PublicConfig>('/public-config');
      setAppName(this.value.app_name);
    } catch {
      this.value = null; // the UI still works with defaults
    }
  },
  get appName(): string {
    return this.value?.app_name ?? 'Pawn.Ink';
  },
  get passwordMin(): number {
    return this.value?.limits.password_min ?? 12;
  },
};
