import { msg } from "@/lib/i18n";
import type { Catalog } from "./sde";
export class Engine {
  worker: Worker;
  next = 0;
  pending = new Map<
    number,
    {
      resolve: (v: unknown) => void;
      reject: (e: Error) => void;
      timer: ReturnType<typeof setTimeout>;
    }
  >();
  constructor() {
    this.worker = new Worker(new URL("./engine.worker.ts", import.meta.url), {
      type: "module",
    });
    this.worker.onmessage = ({ data: m }) => {
      const p = this.pending.get(m.id);
      if (!p) return;
      clearTimeout(p.timer);
      this.pending.delete(m.id);
      if (m.error) p.reject(Error(m.error));
      else p.resolve(m.data);
    };
    this.worker.onerror = () => this.fail(msg("模拟引擎加载失败，请刷新重试"));
  }
  fail(message: string) {
    for (const p of this.pending.values()) {
      clearTimeout(p.timer);
      p.reject(Error(message));
    }
    this.pending.clear();
  }
  call<T>(action: string, body: object = {}): Promise<T> {
    const id = ++this.next;
    return new Promise((resolve, reject) => {
      const timer = setTimeout(() => {
        this.pending.delete(id);
        reject(Error(msg("模拟计算超时，请重试")));
      }, 60000);
      this.pending.set(id, { resolve: (v) => resolve(v as T), reject, timer });
      this.worker.postMessage({ id, action, ...body });
    });
  }
  init() {
    return this.call<Catalog>("init");
  }
  close() {
    this.fail(msg("模拟已关闭"));
    this.worker.terminate();
  }
}
