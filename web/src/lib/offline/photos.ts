/** Réduit une photo avant envoi : 1600 px de côté, 400 Ko visés. Redessiner retire aussi l'EXIF (position GPS). */
export const MAX_SIDE = 1600;
export const TARGET_BYTES = 400 * 1024;

export function scaledSize(width: number, height: number, maxSide = MAX_SIDE): { width: number; height: number } {
  const ratio = Math.min(1, maxSide / Math.max(width, height));
  return { width: Math.round(width * ratio), height: Math.round(height * ratio) };
}

export async function compressPhoto(file: File): Promise<Blob> {
  const bitmap = await createImageBitmap(file);
  const { width, height } = scaledSize(bitmap.width, bitmap.height);
  const canvas = document.createElement("canvas");
  canvas.width = width;
  canvas.height = height;
  const context = canvas.getContext("2d");
  if (!context) throw new Error("Compression de l'image impossible sur cet appareil.");
  context.drawImage(bitmap, 0, 0, width, height);
  bitmap.close();
  let blob: Blob | null = null;
  for (const quality of [0.8, 0.65, 0.5, 0.35]) {
    blob = await new Promise<Blob | null>((resolve) => canvas.toBlob(resolve, "image/jpeg", quality));
    if (blob && blob.size <= TARGET_BYTES) return blob;
  }
  if (!blob) throw new Error("Compression de l'image impossible sur cet appareil.");
  return blob;
}
