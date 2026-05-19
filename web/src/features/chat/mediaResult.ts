export type MediaResult = {
  type: "image" | "video" | "audio";
  url: string;
};

const imageTools = new Set(["generate_image", "edit_image"]);
const videoTools = new Set(["generate_video", "edit_video"]);
const audioTools = new Set(["text_to_speech", "design_voice", "generate_music"]);

const httpMediaRe =
  /https?:\/\/[^\s"'<>]+\.(?:png|jpe?g|gif|webp|svg|mp4|webm|mov|mp3|wav|ogg|flac)(?:\?[^\s"'<>]*)?/i;
const sakerMediaPathRe =
  /(?:\/tmp\/saker-media-|\.saker\/media\/)[^\s"'<>]+\.(?:png|jpe?g|gif|webp|svg|mp4|webm|mov|mp3|wav|ogg|flac)\b/i;
const apiFilesRe =
  /\/api\/files\/[^\s"'<>]+\.(?:png|jpe?g|gif|webp|svg|mp4|webm|mov|mp3|wav|ogg|flac)\b/i;

export function mediaTypeForTool(toolName: string): MediaResult["type"] | null {
  if (imageTools.has(toolName)) return "image";
  if (videoTools.has(toolName)) return "video";
  if (audioTools.has(toolName)) return "audio";
  return null;
}

export function extractMediaResultFromToolResult(
  toolName: string,
  result: string | undefined,
): MediaResult | null {
  const type = mediaTypeForTool(toolName);
  if (!type || !result) return null;

  const httpMatch = result.match(httpMediaRe)?.[0];
  if (httpMatch) return { type, url: httpMatch };

  const apiFilesMatch = result.match(apiFilesRe)?.[0];
  if (apiFilesMatch) return { type, url: apiFilesMatch };

  const sakerPath = result.match(sakerMediaPathRe)?.[0];
  if (sakerPath) return { type, url: `/api/files/${sakerPath}` };

  return null;
}
