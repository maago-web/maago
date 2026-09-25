CREATE TABLE "blog_post" (
    "id" INTEGER PRIMARY KEY AUTOINCREMENT,
    "author_id" INTEGER NOT NULL,
    "title" TEXT NOT NULL CHECK (length("title") <= 200),
    "slug" TEXT NOT NULL CHECK (length("slug") <= 100),
    "status" TEXT NOT NULL DEFAULT 'draft' CHECK ("status" IN ('draft', 'published')),
    "views" INTEGER NOT NULL DEFAULT 0,
    "likes" INTEGER NOT NULL DEFAULT 0,
    "score" REAL DEFAULT 1.5,
    "public" INTEGER NOT NULL DEFAULT 1 CHECK ("public" IN (0, 1)),
    "body" BLOB NOT NULL,
    "published_at" TEXT,
    CONSTRAINT "blog_post_author_id_fkey" FOREIGN KEY ("author_id") REFERENCES "people" ("id") ON DELETE NO ACTION
) STRICT;

CREATE UNIQUE INDEX "blog_post_slug_key" ON "blog_post" ("slug");

CREATE UNIQUE INDEX "blog_post_author_id_slug_key" ON "blog_post" ("author_id", "slug");

CREATE INDEX "blog_post_status_idx" ON "blog_post" ("status");

CREATE INDEX "blog_post_author_id_idx" ON "blog_post" ("author_id");

CREATE TABLE "people" (
    "id" INTEGER PRIMARY KEY AUTOINCREMENT,
    "name" TEXT NOT NULL,
    "parent_id" INTEGER,
    "editor_id" INTEGER NOT NULL,
    "reviewer_id" INTEGER,
    "code" TEXT NOT NULL,
    "order" INTEGER NOT NULL DEFAULT 7,
    "admin" INTEGER NOT NULL DEFAULT 0 CHECK ("admin" IN (0, 1)),
    CONSTRAINT "people_parent_id_fkey" FOREIGN KEY ("parent_id") REFERENCES "people" ("id") ON DELETE NO ACTION,
    CONSTRAINT "people_editor_id_fkey" FOREIGN KEY ("editor_id") REFERENCES "people" ("id") ON DELETE NO ACTION,
    CONSTRAINT "people_reviewer_id_fkey" FOREIGN KEY ("reviewer_id") REFERENCES "people" ("id") ON DELETE NO ACTION
) STRICT;

CREATE UNIQUE INDEX "people_editor_id_key" ON "people" ("editor_id");

CREATE UNIQUE INDEX "people_code_key" ON "people" ("code");

CREATE INDEX "people_reviewer_id_idx" ON "people" ("reviewer_id");

CREATE INDEX "people_code_idx" ON "people" ("code");

CREATE INDEX "people_parent_id_idx" ON "people" ("parent_id");
