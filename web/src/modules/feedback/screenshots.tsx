import { useEffect, useRef, useState, type ClipboardEvent } from "react";
import { ImagePlus, X } from "lucide-react";
import { Button } from "@/components/ui/button";
import { msg } from "@/lib/i18n";
import type { ImageUpload, Screenshot } from "./api";

const limit = msg("截图最多 3 张，仅支持 PNG/JPG");

function ScreenshotPreview({ file, index }: { file: File; index: number }) {
  const image = useRef<HTMLImageElement>(null);
  useEffect(() => {
    const next = URL.createObjectURL(file);
    if (image.current) image.current.src = next;
    return () => URL.revokeObjectURL(next);
  }, [file]);
  return <img ref={image} alt={msg("截图 {0}", index + 1)} />;
}

export function useScreenshots(edited: () => void, disabled: boolean) {
  const [images, setImages] = useState<ImageUpload[]>([]);
  const [error, setError] = useState("");
  const add = (files: File[]) => {
    if (disabled || !files.length) return;
    if (
      images.length + files.length > 3 ||
      files.some(
        (f) => !["image/png", "image/jpeg"].includes(f.type) || f.size < 1,
      )
    ) {
      setError(limit);
      return;
    }
    setError("");
    setImages((current) => [...current, ...files]);
    edited();
  };
  return {
    images,
    busy: false,
    error,
    add,
    clear: () => {
      setImages([]);
      setError("");
    },
    remove: (i: number) => {
      if (!disabled) {
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
              <ScreenshotPreview file={image} index={i} />
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
