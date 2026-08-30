import { SeriesEditor } from "../../series-editor";

export default async function SeriesEditorPage({ params }: { params: Promise<{ seriesID: string }> }) {
  const { seriesID } = await params;
  return <SeriesEditor seriesID={seriesID} />;
}
