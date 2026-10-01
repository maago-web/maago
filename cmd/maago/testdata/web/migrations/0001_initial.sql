CREATE TABLE "site_post" (
    "id" INTEGER PRIMARY KEY AUTOINCREMENT,
    "title" TEXT NOT NULL CHECK (length("title") <= 200)
) STRICT;
