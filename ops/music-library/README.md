# Background music library

5 CC0 tracks tagged by mood/genre/tempo. The LLM picker reads `manifest.json`
when the run's `music` step fires and selects one `id`. The worker then
references `object_key` as the music asset for the assemble step.

## Seeding

For the first iteration, the library lives only as the manifest above —
upload the actual MP3 files into MinIO under the keys listed in `object_key`:

```bash
# Example: download a CC0 track from Pixabay and push to MinIO
mc cp my-energetic-pop.mp3 minio-local/webcomics/library/music/energetic-pop-01.mp3
```

Use Pixabay (`https://pixabay.com/music/`, CC0) or Freesound (filter
`license:CC0`) and pick tracks that match the descriptors here. Loop-friendly
tracks need a clean ending.

## Adding new tracks

1. Pick a CC0 source. Verify license at the source page.
2. Upload to MinIO under `library/music/<id>.mp3`.
3. Append an entry to `manifest.json` with at least: id, title, object_key,
   duration_s, mood, genre, tempo, license, source.
4. Restart the music worker so it re-reads the manifest.

## How the LLM picker chooses

The Python music handler:

1. Reads `manifest.json`.
2. Sends a short prompt to OpenRouter: script text + manifest mood/genre/tempo.
3. Receives `{track_id, reasoning}` JSON.
4. Validates `track_id` is in the manifest; falls back to a random track if not.
5. Publishes the chosen `object_key` to the assemble step.

Per-project override: `project.defaults.music.preferred_mood` skips the LLM
and picks the first matching track.

## Renderer mixing

`renderer-node/Comic.tsx` mixes music + voiceover:

- Voice at 1.0 volume.
- Music at 0.3 volume when voice is present (manual ducking).
- Music at 0.6 volume when no voice asset.

These constants live in `Comic.tsx` and can be tuned per-style later.
