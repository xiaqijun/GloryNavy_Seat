import { expect, it } from "vitest";
import { english } from "@/lib/locales/en";
import { frontendModules } from "@/app/modules";
import guide from "./help-content.json";

it("keeps help bookmarks, business destinations and translations valid", () => {
  const ids = guide.articles.map((article) => article.id);
  expect(new Set(ids).size).toBe(ids.length);
  const paths = new Set(
    frontendModules.flatMap((module) => module.pages.map((page) => page.path)),
  );
  const messages = [guide.intro, ...guide.groups];
  for (const article of guide.articles) {
    expect(guide.groups).toContain(article.group);
    expect(article.id).toMatch(/^[a-z][a-z-]*$/);
    messages.push(article.title, article.summary);
    for (const link of article.links) {
      expect(paths.has(link.path), `${article.id}: ${link.path}`).toBe(true);
      messages.push(link.label);
    }
    for (const section of article.sections)
      messages.push(section.title, ...section.steps, ...section.notes);
    for (const question of article.questions)
      messages.push(question.title, question.answer);
  }
  for (const message of messages) {
    expect(english[message], message).toBeTruthy();
    expect(english[message], message).not.toMatch(/[\u3400-\u9fff]/);
  }
});
