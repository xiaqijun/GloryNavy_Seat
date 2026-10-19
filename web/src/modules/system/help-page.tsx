import { useEffect, useRef, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { ArrowRight, BookOpen, Printer, Search } from "lucide-react";
import { SearchInput } from "@/components/search-input";
import { IconAction } from "@/components/ui/icon-action";
import { Select } from "@/components/ui/select";
import { getLocale, msg } from "@/lib/i18n";
import guide from "./help-content.json";
import "./help.css";

const normalize = (text: string) =>
  text.normalize("NFKC").toLocaleLowerCase(getLocale());

export default function HelpPage() {
  const [params, setParams] = useSearchParams();
  const [query, setQuery] = useState("");
  const article =
    guide.articles.find((item) => item.id === params.get("topic")) ??
    guide.articles[0];
  const heading = useRef<HTMLHeadingElement>(null);
  const lastTopic = useRef(article.id);
  const focusRequested = useRef(false);
  const terms = normalize(query).trim().split(/\s+/).filter(Boolean);
  const searching = terms.length > 0;
  const results = guide.articles.flatMap((item) => {
    const paragraphs = [
      item.title,
      item.summary,
      ...item.sections.flatMap((section) => [
        section.title,
        ...section.steps,
        ...section.notes,
      ]),
      ...item.questions.flatMap((question) => [
        question.title,
        question.answer,
      ]),
    ].map((text) => msg(text));
    const fullText = normalize(paragraphs.join(" "));
    if (!terms.every((term) => fullText.includes(term))) return [];
    const excerpt = paragraphs
      .slice(2)
      .find((text) => terms.some((term) => normalize(text).includes(term)));
    return [{ item, excerpt: excerpt ?? msg(item.summary) }];
  });

  useEffect(() => {
    if (
      !searching &&
      (lastTopic.current !== article.id || focusRequested.current)
    ) {
      heading.current?.focus({ preventScroll: true });
      heading.current?.scrollIntoView({ block: "start", behavior: "instant" });
      focusRequested.current = false;
    }
    lastTopic.current = article.id;
  }, [article.id, searching]);

  const topicUrl = (id: string) => {
    const next = new URLSearchParams(params);
    next.set("topic", id);
    return `?${next.toString()}`;
  };
  const selectTopic = () => {
    focusRequested.current = true;
    setQuery("");
  };

  return (
    <section className="help-page">
      <header className="help-heading">
        <div>
          <h1>{msg("使用帮助")}</h1>
          <p>{msg(guide.intro)}</p>
        </div>
        {!searching && (
          <IconAction label={msg("打印本节")} onClick={() => window.print()}>
            <Printer size={18} />
          </IconAction>
        )}
      </header>
      <div className="help-search">
        <label htmlFor="help-search">
          <Search size={18} aria-hidden="true" />
          {msg("搜索帮助")}
        </label>
        <SearchInput
          id="help-search"
          value={query}
          onChange={(event) => setQuery(event.target.value)}
          onClear={() => setQuery("")}
          placeholder={msg("搜索操作、问题或关键词")}
          autoComplete="off"
        />
      </div>
      <div className="help-layout">
        <nav className="help-directory" aria-label={msg("帮助目录")}>
          {guide.groups.map((group) => (
            <div className="help-topic-group" key={group}>
              <h2>{msg(group)}</h2>
              {guide.articles
                .filter((item) => item.group === group)
                .map((item) => (
                  <Link
                    key={item.id}
                    to={topicUrl(item.id)}
                    aria-current={
                      !searching && item.id === article.id ? "page" : undefined
                    }
                    onClick={(event) => {
                      if (
                        !event.ctrlKey &&
                        !event.metaKey &&
                        !event.shiftKey &&
                        !event.altKey
                      )
                        selectTopic();
                    }}
                  >
                    {msg(item.title)}
                  </Link>
                ))}
            </div>
          ))}
        </nav>
        <div className="help-mobile-directory">
          <Select
            label={msg("帮助目录")}
            value={article.id}
            options={guide.articles.map((item) => ({
              value: item.id,
              label: msg(item.title),
              group: msg(item.group),
            }))}
            onValueChange={(id) => {
              selectTopic();
              setParams((previous) => {
                const next = new URLSearchParams(previous);
                next.set("topic", id);
                return next;
              });
            }}
          />
        </div>
        {searching ? (
          <section className="help-results" aria-label={msg("搜索结果")}>
            <h2 role="status" aria-live="polite">
              {msg("找到 {0} 个主题", results.length)}
            </h2>
            {results.length ? (
              results.map(({ item, excerpt }) => (
                <Link
                  className="help-result"
                  to={topicUrl(item.id)}
                  key={item.id}
                  onClick={(event) => {
                    if (
                      !event.ctrlKey &&
                      !event.metaKey &&
                      !event.shiftKey &&
                      !event.altKey
                    )
                      selectTopic();
                  }}
                >
                  <span className="help-result-title">
                    {msg(item.title)}
                    <ArrowRight size={18} aria-hidden="true" />
                  </span>
                  <p>{excerpt}</p>
                </Link>
              ))
            ) : (
              <div className="help-empty">
                <BookOpen size={24} aria-hidden="true" />
                <p>{msg("试试更短的关键词，或从目录选择主题。")}</p>
              </div>
            )}
          </section>
        ) : (
          <article
            className="help-article"
            aria-labelledby="help-article-title"
          >
            <header className="help-article-heading">
              <h2 id="help-article-title" tabIndex={-1} ref={heading}>
                {msg(article.title)}
              </h2>
              <p>{msg(article.summary)}</p>
              {article.links.length > 0 && (
                <div className="help-business-links">
                  {article.links.map((link) => (
                    <Link key={link.path} to={link.path}>
                      {msg(link.label)}
                      <ArrowRight size={16} aria-hidden="true" />
                    </Link>
                  ))}
                </div>
              )}
            </header>
            {article.sections.map((section) => (
              <section className="help-section" key={section.title}>
                <h3>{msg(section.title)}</h3>
                {section.steps.length > 0 && (
                  <ol>
                    {section.steps.map((step) => (
                      <li key={step}>{msg(step)}</li>
                    ))}
                  </ol>
                )}
                {section.notes.length > 0 && (
                  <ul className="help-notes">
                    {section.notes.map((note) => (
                      <li key={note}>{msg(note)}</li>
                    ))}
                  </ul>
                )}
              </section>
            ))}
            {article.questions.map((question) => (
              <section
                className="help-section help-question"
                key={question.title}
              >
                <h3>{msg(question.title)}</h3>
                <p>{msg(question.answer)}</p>
              </section>
            ))}
            <footer className="help-article-footer">
              {msg(
                "更新于 {0}",
                new Date(`${guide.updated}T00:00:00Z`).toLocaleDateString(
                  getLocale(),
                  {
                    timeZone: "UTC",
                    year: "numeric",
                    month: "short",
                    day: "numeric",
                  },
                ),
              )}
            </footer>
          </article>
        )}
      </div>
    </section>
  );
}
