import { useRef, useState, type ClipboardEvent } from "react";
import { ImagePlus, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { msg } from "@/lib/i18n";
import type { ImageUpload, Screenshot } from "./api";

const limit = msg("截图最多 3 张，仅支持 PNG/JPG，每张不超过 2 MB");
const dataURL = (image: ImageUpload) =>
  `data:${image.mime};base64,${image.content}`;
const readFile = (file: File) =>
  new Promise<ImageUpload>((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () =>
      resolve({
        mime: file.type,
        content: String(reader.result).split(",")[1],
      });
    reader.onerror = reader.onabort = () =>
      reject(new Error(msg("截图读取失败，请重新选择")));
    reader.readAsDataURL(file);
  });

export function useScreenshots(edited: () => void, disabled: boolean) {
  const [images, setImages] = useState<ImageUpload[]>([]);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const reading = useRef(false);
  const add = async (files: File[]) => {
    if (disabled || reading.current || !files.length) return;
    if (
      images.length + files.length > 3 ||
      files.some(
        (f) =>
          !["image/png", "image/jpeg"].includes(f.type) ||
          f.size < 1 ||
          f.size > 2_000_000,
      )
    ) {
      setError(limit);
      return;
    }
    reading.current = true;
    setBusy(true);
    setError("");
    try {
      const next = await Promise.all(files.map(readFile));
      setImages((current) => [...current, ...next]);
      edited();
    } catch (e) {
      setError(
        e instanceof Error ? e.message : msg("截图读取失败，请重新选择"),
      );
    } finally {
      reading.current = false;
      setBusy(false);
    }
  };
  return {
    images,
    busy,
    error,
    add,
    clear: () => {
      setImages([]);
      setError("");
    },
    remove: (i: number) => {
      if (!disabled && !reading.current) {
        setImages((v) => v.filter((_, n) => n !== i));
        setError("");
        edited();
      }
    },
    paste: (e: ClipboardEvent) => {
      const files = Array.from(e.clipboardData.files).filter((f) =>
        f.type.startsWith("image/"),
      );
      if (files.length) {
        e.preventDefault();
        void add(files);
      }
    },
  };
}

export function ScreenshotPicker({
  value,
  disabled,
}: {
  value: ReturnType<typeof useScreenshots>;
  disabled: boolean;
}) {
  const input = useRef<HTMLInputElement>(null);
  return (
    <div className="feedback-screenshots">
      <div className="feedback-image-toolbar">
        <Button
          type="button"
          variant="outline"
          disabled={disabled || value.busy || value.images.length >= 3}
          onClick={() => input.current?.click()}
          title={limit}
        >
          <ImagePlus size={18} />
          {value.busy ? msg("正在读取") : msg("添加截图")}
        </Button>
        {value.images.length > 0 && <span>{value.images.length}/3</span>}
        <input
          ref={input}
          type="file"
          className="feedback-file-input"
          aria-label={msg("添加截图")}
          accept="image/png,image/jpeg"
          multiple
          disabled={disabled || value.busy}
          onChange={(e) => {
            void value.add(Array.from(e.target.files || []));
            e.target.value = "";
          }}
        />
      </div>
      {value.images.length > 0 && (
        <div className="feedback-images">
          {value.images.map((image, i) => (
            <div className="feedback-image-draft" key={i}>
              <img src={dataURL(image)} alt={msg("截图 {0}", i + 1)} />
              <button
                type="button"
                aria-label={msg("移除截图 {0}", i + 1)}
                disabled={disabled || value.busy}
                onClick={() => value.remove(i)}
              >
                <X size={18} />
              </button>
            </div>
          ))}
        </div>
      )}
      {value.error && <p role="alert">{value.error}</p>}
    </div>
  );
}

export function ScreenshotGallery({
  ticket,
  images,
}: {
  ticket: string;
  images?: Screenshot[];
}) {
  if (!images?.length) return null;
  return (
    <div className="feedback-images">
      {images.map((image, i) => {
        const url = `/api/v1/feedback/tickets/${ticket}/images/${image.id}`;
        return (
          <a
            key={image.id}
            href={url}
            target="_blank"
            rel="noreferrer"
            aria-label={msg("查看截图 {0}", i + 1)}
          >
            <img src={url} alt={msg("截图 {0}", i + 1)} loading="lazy" />
          </a>
        );
      })}
    </div>
  );
}
