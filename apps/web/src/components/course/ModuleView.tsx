import type {
  BookData,
  FolderData,
  LabelData,
  ModuleContent,
  PageData,
  ResourceData,
  UrlData,
} from "@/lib/api";

function formatBytes(bytes: number) {
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
  return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
}

const KIND_LABEL: Record<string, string> = {
  page: "Page",
  url: "Link",
  resource: "File",
  folder: "Folder",
  book: "Book",
};

function Entry({
  kind,
  title,
  children,
}: {
  kind: string;
  title: React.ReactNode;
  children?: React.ReactNode;
}) {
  return (
    <article className="flex flex-col gap-2 py-5">
      <span className="meta">{KIND_LABEL[kind] ?? kind}</span>
      <h3 className="text-xl">{title}</h3>
      {children}
    </article>
  );
}

/** Numbers chapters like a table of contents: 1, 1.1, 1.2, 2, … */
function numberChapters(chapters: BookData["chapters"]) {
  let main = 0;
  let sub = 0;
  return chapters.map((chapter) => {
    if (chapter.subchapter && main > 0) {
      sub += 1;
      return { chapter, label: `${main}.${sub}` };
    }
    main += 1;
    sub = 0;
    return { chapter, label: String(main) };
  });
}

const bodyText = "prose-measure whitespace-pre-wrap text-ink";

export function ModuleView({
  module,
  extra,
  manage,
}: {
  module: ModuleContent;
  extra?: React.ReactNode;
  /** Management controls shown under the module, for people who can edit it. */
  manage?: React.ReactNode;
}) {
  const body = renderModule(module, extra);
  const notes: string[] = [];
  if (!module.visible) notes.push("Hidden from students");
  if (module.available_from) notes.push(`Opens ${new Date(module.available_from).toLocaleString()}`);
  if (module.available_until) notes.push(`Closes ${new Date(module.available_until).toLocaleString()}`);

  return (
    <div className="flex flex-col gap-2">
      {module.restricted ? (
        <article className="flex flex-col gap-1 py-5 text-muted">
          <span className="meta">Not available yet</span>
          <h3 className="text-xl">{module.module_type}</h3>
        </article>
      ) : (
        body
      )}
      {module.intro && !module.restricted ? (
        <p className="prose-measure text-muted">{module.intro}</p>
      ) : null}
      {notes.length > 0 ? <p className="meta">{notes.join(" · ")}</p> : null}
      {manage ? <div className="pb-3">{manage}</div> : null}
    </div>
  );
}

function renderModule(module: ModuleContent, extra?: React.ReactNode) {
  if (!module.data) return null;
  switch (module.module_type) {
    case "label": {
      const data = module.data as LabelData;
      return <p className="prose-measure py-4 text-lg">{data.content}</p>;
    }

    case "page": {
      const data = module.data as PageData;
      return (
        <Entry kind="page" title={data.title}>
          <p className={bodyText}>{data.content}</p>
        </Entry>
      );
    }

    case "url": {
      const data = module.data as UrlData;
      return (
        <Entry
          kind="url"
          title={
            <a href={data.url} target="_blank" rel="noreferrer" className="link">
              {data.title}
            </a>
          }
        >
          {data.description ? <p className="prose-measure text-muted">{data.description}</p> : null}
        </Entry>
      );
    }

    case "resource": {
      const data = module.data as ResourceData;
      return (
        <Entry
          kind="resource"
          title={
            <a href={data.download_url} className="link">
              {data.title}
            </a>
          }
        >
          <p className="meta break-all">
            {data.file_name} · {formatBytes(data.file_size)}
          </p>
        </Entry>
      );
    }

    case "folder": {
      const data = module.data as FolderData;
      return (
        <Entry kind="folder" title={data.title}>
          {data.description ? <p className="prose-measure text-muted">{data.description}</p> : null}
          {data.files.length > 0 ? (
            <ul className="flex flex-col">
              {data.files.map((file) => (
                <li
                  key={file.id}
                  className="flex flex-wrap items-baseline justify-between gap-x-4 border-t border-rule py-2 first:border-t-0"
                >
                  <a href={file.download_url} className="link min-w-0 break-all">
                    {file.dir_path && file.dir_path !== "/" ? `${file.dir_path.slice(1)}/` : ""}
                    {file.file_name}
                  </a>
                  <span className="meta">{formatBytes(file.file_size)}</span>
                </li>
              ))}
            </ul>
          ) : (
            <p className="text-sm text-muted">No files yet.</p>
          )}
          {extra}
        </Entry>
      );
    }

    case "book": {
      const data = module.data as BookData;
      return (
        <Entry kind="book" title={data.title}>
          {data.intro ? <p className="prose-measure text-muted">{data.intro}</p> : null}
          {data.chapters.length > 0 ? (
            <ol className="flex flex-col gap-5 border-t border-rule pt-4">
              {numberChapters(data.chapters).map(({ chapter, label }) => (
                <li
                  key={chapter.id}
                  className={`flex flex-col gap-1 ${chapter.subchapter ? "ml-8" : ""}`}
                >
                  <h4 className="font-display text-lg font-medium">
                    <span className="meta mr-3">{label}</span>
                    {chapter.title}
                    {chapter.hidden ? <span className="meta ml-3">Hidden</span> : null}
                  </h4>
                  <p className={bodyText}>{chapter.content}</p>
                </li>
              ))}
            </ol>
          ) : (
            <p className="text-sm text-muted">No chapters yet.</p>
          )}
          {extra}
        </Entry>
      );
    }

    default:
      return null;
  }
}
