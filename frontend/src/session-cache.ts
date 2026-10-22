/** Bounded, session-only data cache. Never persisted as real-time market data. */
export class SessionCache<T> {
  private entries = new Map<string, { value: T; updatedAt: number }>();
  constructor(private capacity: number) {}
  get(key: string) {
    const entry = this.entries.get(key);
    if (entry) { this.entries.delete(key); this.entries.set(key, entry); }
    return entry;
  }
  set(key: string, value: T) {
    this.entries.delete(key);
    this.entries.set(key, { value, updatedAt: Date.now() });
    if (this.entries.size > this.capacity) this.entries.delete(this.entries.keys().next().value!);
  }
}
