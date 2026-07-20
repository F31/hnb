import { rename, rm } from "node:fs/promises";

export async function replaceDirectoryAtomically(source, target, backup) {
  await rm(backup, { recursive: true, force: true });
  let hadTarget = true;
  try {
    await rename(target, backup);
  } catch (error) {
    if (error.code !== "ENOENT") throw error;
    hadTarget = false;
  }

  try {
    await rename(source, target);
    if (hadTarget) await rm(backup, { recursive: true, force: true });
  } catch (error) {
    if (hadTarget) await rename(backup, target);
    throw error;
  }
}
