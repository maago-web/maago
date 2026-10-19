CREATE TABLE "site_tag" (
    "id" INTEGER PRIMARY KEY AUTOINCREMENT,
    "name" TEXT NOT NULL CHECK (length("name") <= 50),
    "kind" TEXT NOT NULL CHECK (length("kind") <= 50),
    "code" TEXT NOT NULL CHECK (length("code") <= 50)
) STRICT;

CREATE UNIQUE INDEX "site_tag_name_key" ON "site_tag" ("name");

CREATE UNIQUE INDEX "site_tag_kind_code_key" ON "site_tag" ("kind", "code");
