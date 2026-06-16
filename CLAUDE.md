The goal of this project is to develop comprehensive business management software.

Technology stack:
Postgres for the database.
Go for the back end.
TypeScript and React for the front end.
Python as necessary for ancillary scripts.

Schema design:
Where a good natural key presents itself, such as the ISO 2 letter country code, it shall be used.
Where a synthetic key is needed, it shall be an auto-incrementing integer. This forgoes the distributed merge advantages of a UUID key, but gains performance and ease of debugging.

Version control:
Commit directly to the default branch. Do not create feature branches.
