# EHDB

![GitHub License](https://img.shields.io/github/license/Slinet6056/ehdb)
![GitHub go.mod Go version](https://img.shields.io/github/go-mod/go-version/Slinet6056/ehdb)

A high-performance E-Hentai/ExHentai gallery database with RESTful API and automated synchronization tools.

This project is inspired by [ccloli/e-hentai-db](https://github.com/ccloli/e-hentai-db) and reimplemented with PostgreSQL, achieving **10-100x query performance improvements** over the original MySQL implementation. It also uses live database dumps from [URenko/e-hentai-db](https://github.com/URenko/e-hentai-db) for quick initial setup with the latest data. Special thanks to both projects!

## Quick Start

### Prerequisites

- Go 1.24 or higher
- PostgreSQL (17+ required for pre-built database import)
- Docker & Docker Compose (optional)

### Installation

#### Using Docker Compose

```bash
# Download configuration files
mkdir ehdb && cd ehdb
wget https://raw.githubusercontent.com/Slinet6056/ehdb/master/docker-compose.yml
wget https://raw.githubusercontent.com/Slinet6056/ehdb/master/config.example.yaml

# Configure
cp config.example.yaml config.yaml
# Edit docker-compose.yml and config.yaml with your settings

# Start services
docker compose up -d
```

#### Building from Source

```bash
# Clone and build
git clone https://github.com/Slinet6056/ehdb.git
cd ehdb
go build -o bin/ehdb-api ./cmd/api
go build -o bin/ehdb-sync ./cmd/sync

# Configure
cp config.example.yaml config.yaml
# Edit config.yaml with your settings

# Run
./bin/ehdb-api -config config.yaml -scheduler
```

### Database Setup

You need to set up a PostgreSQL database before using EHDB.

#### Import Pre-built Database

We provide two types of pre-built database dumps:

**1. Live Sync Database (Recommended)**

Incrementally synchronized database updated every 6 hours with the latest galleries and torrents:

```bash
# Download the dump file
wget https://github.com/Slinet6056/ehdb/releases/download/live/ehentai_db.dump

# Create database
createdb -U postgres ehentai_db

# Import (requires PostgreSQL 17+)
pg_restore -U postgres -d ehentai_db --no-owner --no-privileges -v ehentai_db.dump
```

> **Note**: The live database is automatically updated every 6 hours via [sync workflow](.github/workflows/sync-database.yml) and deep resynced monthly via [monthly resync workflow](.github/workflows/monthly-resync.yml).

**2. Nightly Migration Database (Alternative)**

Full database migrated from upstream MySQL source, updated daily:

```bash
# Download the dump file
wget https://github.com/Slinet6056/ehdb/releases/download/nightly/ehentai_db.dump

# Create database
createdb -U postgres ehentai_db

# Import (requires PostgreSQL 16+)
pg_restore -U postgres -d ehentai_db --no-owner --no-privileges -v ehentai_db.dump
```

> **Note**: The nightly database dump is automatically updated daily via [migration workflow](.github/workflows/database-migration.yml).

#### Migrate from MySQL

If you have an existing MySQL database, see the [Migration Guide](migration/README.md) for detailed migration instructions using pgloader.

## Usage

### API Server

Start the API server (provides REST API endpoints only):

```bash
./bin/ehdb-api -config config.yaml
```

Start the API server with scheduler enabled (automatically executes scheduled sync tasks):

```bash
./bin/ehdb-api -config config.yaml -scheduler
```

**Parameters:**

- `-config`: Config file path (optional, default: `config.yaml`)
- `-scheduler`: Enable automatic task scheduler for periodic syncing (optional)

### Sync Tool

The `ehdb-sync` command provides multiple synchronization operations:

#### Sync Latest Galleries

Synchronize the latest galleries from E-Hentai/ExHentai (crawls from the front page until reaching already synced content):

```bash
./bin/ehdb-sync sync -host e-hentai.org -offset 2
```

**Parameters:**

- `-config`: Config file path (optional, default: `config.yaml`)
- `-host`: Specify the site - `e-hentai.org` or `exhentai.org` (optional, overrides config)
- `-offset`: Re-crawl galleries from the last N hours in the database (optional, default: 0, useful for refreshing recently updated data)

#### Resync Recent Galleries

Query galleries posted within the last N hours from the database and re-fetch their metadata to update changed information (ratings, tags, etc.):

```bash
./bin/ehdb-sync resync -hours 24
```

**Parameters:**

- `-config`: Config file path (optional, default: `config.yaml`)
- `-hours`: Specify how many hours back to query and re-sync (optional, default: 24)

#### Fetch Specific Galleries

Fetch specific galleries by GID/Token:

```bash
./bin/ehdb-sync fetch 123456/abcdef0123 234567/bcdef01234
```

Batch import from a file:

```bash
./bin/ehdb-sync fetch -file galleries.txt
```

**Parameters:**

- `-config`: Config file path (optional, default: `config.yaml`)
- `-file`: File containing `gid/token` pairs, one per line (optional, if not specified, read from command line arguments)

#### Sync Torrents

Synchronize torrent information from the torrent list page (crawls until reaching existing torrents or specified page limit):

```bash
./bin/ehdb-sync torrent-sync
./bin/ehdb-sync torrent-sync -pages 5 -status completed -search keyword
```

**Parameters:**

- `-config`: Config file path (optional, default: `config.yaml`)
- `-host`: Specify the site - `e-hentai.org` or `exhentai.org` (optional, overrides config)
- `-pages`: Number of pages to crawl (optional, default: 0 = crawl until reaching existing torrents)
- `-status`: Filter by torrent status, e.g., `completed`, `unseeded` (optional)
- `-search`: Search keyword to filter torrents (optional)

#### Import Torrents

Scan all galleries in the database and import their torrent information from gallery detail pages (heavy operation):

```bash
./bin/ehdb-sync torrent-import
```

**Parameters:**

- `-config`: Config file path (optional, default: `config.yaml`)
- `-host`: Specify the site - `e-hentai.org` or `exhentai.org` (optional, overrides config)

> **Warning**: This is a heavy operation that will scan all galleries in the database.

#### Mark Replaced Galleries

Scan and mark galleries that have been replaced by newer versions:

```bash
./bin/ehdb-sync mark-replaced
```

**Parameters:**

- `-config`: Config file path (optional, default: `config.yaml`)

## API Endpoints

All list endpoints support two pagination modes:

- **Page-based**: `?page=1&limit=25` - Good for shallow pagination (first few pages)
- **Cursor-based**: `?cursor=timestamp,gid&limit=25` - Best for deep pagination (constant performance)

> **Note**: Cursor format is `timestamp,gid`. The API always returns `next_cursor` in responses for easy pagination.

### Gallery Operations

#### Get Gallery by GID and Token

```
GET /api/gallery/:gid/:token
GET /api/g/:gid/:token
```

**Path Parameters:**

- `gid` - Gallery ID (numeric)
- `token` - Gallery token (10-character hex string)

**Example:**

```
GET /api/gallery/123456/abcdef0123
```

### Category Operations

#### Get Galleries by Category

```
GET /api/category/:category
GET /api/cat/:category
```

**Path Parameters:**

- `category` - Category name, comma-separated names, or numeric bit mask

**Category Names:**

- `Doujinshi`, `Manga`, `Artist CG`, `Game CG`, `Western`, `Non-H`, `Image Set`, `Cosplay`, `Asian Porn`, `Misc`, `Private`

**Category Bit Mask (can be combined using bitwise OR):**

| Bit Value | Binary Position | Category   |
| --------- | --------------- | ---------- |
| 1         | 2^0             | Misc       |
| 2         | 2^1             | Doujinshi  |
| 4         | 2^2             | Manga      |
| 8         | 2^3             | Artist CG  |
| 16        | 2^4             | Game CG    |
| 32        | 2^5             | Image Set  |
| 64        | 2^6             | Cosplay    |
| 128       | 2^7             | Asian Porn |
| 256       | 2^8             | Non-H      |
| 512       | 2^9             | Western    |
| 1024      | 2^10            | Private    |

**Bit Mask Examples:**

_Single Category:_

- `2` - Only Doujinshi
- `4` - Only Manga
- `512` - Only Western

_Multiple Categories (combine with addition or bitwise OR):_

- `6` (2+4) - Doujinshi + Manga
- `10` (2+8) - Doujinshi + Artist CG
- `774` (2+4+8+256+512) - Doujinshi + Manga + Artist CG + Non-H + Western

_All Non-Private Categories:_

- `1023` (1+2+4+8+16+32+64+128+256+512) - All categories except Private
- `2047` (1023+1024) - All categories including Private

_Exclude Categories (negative value, uses XOR with 2047):_

- `-2` - All categories EXCEPT Doujinshi (equivalent to 2045)
- `-6` - All categories EXCEPT Doujinshi and Manga (equivalent to 2041)
- `-1024` - All categories EXCEPT Private (equivalent to 1023)

**Query Parameters:**

- `page` - Page number (optional, default: 1)
- `limit` - Results per page (optional, default: 25, max: configurable)
- `cursor` - Cursor for cursor-based pagination (optional)

**Usage Examples:**

```
# By category name (single)
GET /api/category/Doujinshi

# By category names (multiple, comma-separated)
GET /api/category/Doujinshi,Manga

# By bit mask (single category)
GET /api/category/2

# By bit mask (multiple categories)
GET /api/category/774

# By bit mask (all non-private)
GET /api/category/1023

# Exclude categories (negative bit mask)
GET /api/category/-2

# With pagination
GET /api/cat/Doujinshi?page=2&limit=50

# With cursor pagination
GET /api/cat/1023?cursor=1704067200,123456&limit=25
```

### List Operations

#### List All Galleries

```
GET /api/list
```

**Query Parameters:**

- `page` - Page number (optional, default: 1, for page-based pagination)
- `limit` - Results per page (optional, default: 25, max: configurable)
- `cursor` - Cursor for cursor-based pagination (optional, format: `timestamp,gid`)

**Examples:**

```
GET /api/list?page=1&limit=25
GET /api/list?cursor=1704067200,123456&limit=25
```

#### Search Galleries

```
GET /api/search
```

**Query Parameters:**

_Search & Filters:_

- `keyword` - Search keyword with advanced syntax (optional, see [Search Syntax](#search-syntax))
- `category` - Category filter (optional, see [Category Operations](#category-operations))
- `expunged` - Include expunged galleries (optional, 0=exclude, 1=include, default: 0)
- `removed` - Include removed galleries (optional, 0=exclude, 1=include, default: 0)
- `replaced` - Include replaced galleries (optional, 0=exclude, 1=include, default: 0)
- `minpage` - Minimum page count (optional, default: 0)
- `maxpage` - Maximum page count (optional, default: 0)
- `minrating` - Minimum rating (optional, 0-5, default: 0)
- `mindate` - Minimum posted date (optional, Unix timestamp)
- `maxdate` - Maximum posted date (optional, Unix timestamp)

_Pagination:_

- `page` - Page number (optional, default: 1)
- `limit` - Results per page (optional, default: 10, max: configurable)
- `cursor` - Cursor for cursor-based pagination (optional)

**Examples:**

```
GET /api/search?keyword=full_color&category=Doujinshi&minpage=20
GET /api/search?keyword=female:elf%20-male:yaoi&minrating=4.5
GET /api/search?keyword=~artist:aaa%20~artist:bbb&cursor=1704067200,123456&limit=25
```

### Tag Operations

#### Get Galleries by Tag

```
GET /api/tag/:tag
```

**Path Parameters:**

- `tag` - Tag name (supports comma-separated multiple tags for AND search)

**Query Parameters:**

- `page` - Page number (optional, default: 1)
- `limit` - Results per page (optional, default: 25, max: configurable)
- `cursor` - Cursor for cursor-based pagination (optional)

**Examples:**

```
GET /api/tag/o:full%20color
GET /api/tag/female:elf,male:sole%20male
GET /api/tag/f:big%20breasts?cursor=1704067200,123456&limit=50
```

### Uploader Operations

#### Get Galleries by Uploader

```
GET /api/uploader/:uploader
```

**Path Parameters:**

- `uploader` - Uploader name

**Query Parameters:**

- `page` - Page number (optional, default: 1)
- `limit` - Results per page (optional, default: 25, max: configurable)
- `cursor` - Cursor for cursor-based pagination (optional)

**Example:**

```
GET /api/uploader/someuser?page=1&limit=25
```

## Search Syntax

The search API supports E-Hentai-style search syntax ([reference](https://ehwiki.org/wiki/Gallery_Searching)).

**Important**:

- Terms with colon format (`namespace:tag`) are treated as **tag searches**
- Terms without colon are treated as **title searches** (searches both English and Japanese titles)

### Operators

| Operator         | Syntax                      | Description                                             | Example                                         |
| ---------------- | --------------------------- | ------------------------------------------------------- | ----------------------------------------------- |
| **Quotation**    | `"phrase"` or `ns:"phrase"` | Exact phrase (for titles or multi-word tags)            | `"comic aun"`, `character:"dark magician girl"` |
| **Tag Search**   | `namespace:tag`             | Search by tag with namespace (must contain colon)       | `female:elf`, `artist:aaa`                      |
| **Title Search** | `term`                      | Search in titles (no colon, searches both EN/JP titles) | `pokemon`, `tankobon`                           |
| **Wildcard**     | `term*` or `*term`          | Wildcard matching (titles only)                         | `school*`, `*girl`                              |
| **Exclude**      | `-term` or `-namespace:tag` | Exclude results                                         | `-furry`, `-male:yaoi`                          |
| **OR**           | `~term1 ~term2`             | Match any of the terms                                  | `~female:elf ~female:fairy`                     |
| **Exact Tag**    | `namespace:tag$`            | Exact tag match (prevent partial matches)               | `female:wolf$` (won't match "wolf girl")        |

### Tag Namespaces

Supported namespaces (compatible with E-Hentai):

- `female:` or `f:` - Female tags
- `male:` or `m:` - Male tags
- `mixed:` or `x:` - Mixed tags
- `other:` or `o:` - Other tags
- `artist:` or `a:` - Artist tags
- `group:` or `g:` or `circle:` - Group/circle tags
- `parody:` or `p:` or `series:` - Parody/series tags
- `character:` or `c:` or `char:` - Character tags
- `language:` or `l:` or `lang:` - Language tags
- `location:` or `loc:` - Location tags
- `reclass:` or `r:` - Reclass tags
- `cosplayer:` or `cos:` - Cosplayer tags

### Search Examples

**Tag Search (with colon):**

```
keyword=female:elf                      # Prefix match: matches "female:elf", "female:elf ears", etc.
keyword=female:elf$                     # Exact match: matches only "female:elf"
keyword=f:big                           # Prefix match: expands to all "female:big*" tags
keyword=f:elf m:sole                    # Multiple prefix tags (AND)
keyword=character:"dark magician girl"  # Prefix match with spaces (must use quotes)
keyword=c:"dark magician girl$"         # Exact match with spaces
```

**Title Search (without colon):**

```
keyword="original work"         # Exact phrase in title
keyword=pokemon                 # Word in title (searches both EN/JP)
```

**Wildcard Search (titles only):**

```
keyword=school*                 # Titles starting with "school"
keyword=*girl                   # Titles ending with "girl"
```

**Exclude Terms:**

```
keyword=pokemon -furry           # Title contains "pokemon" but not "furry"
keyword=female:elf -male:yaoi    # Has tag female:elf but not male:yaoi
keyword="comic aun" -2007 -2008  # Title has "comic aun" but not "2007" or "2008"
```

**OR Search (match any):**

```
keyword=~yaoi ~furry                    # Title contains "yaoi" OR "furry"
keyword=~female:elf ~female:fairy       # Tag female:elf OR female:fairy
```

**Tag Prefix vs Exact Match:**

```
# Prefix match (without $)
keyword=female:wolf                             # Matches "female:wolf", "female:wolf girl", "female:wolf ears", etc.
keyword=f:big                                   # Matches "female:big breasts", "female:big ass", "female:big areolae", etc.

# Exact match (with $)
keyword=female:wolf$                            # Matches ONLY "female:wolf"
keyword=f:"big breasts$"                        # Matches ONLY "female:big breasts"
```

**Mixed Tag and Title Search:**

```
keyword=female:elf "full color" -male:yaoi              # Prefix tag + title phrase - prefix tag
keyword=female:elf$ "full color" -male:yaoi$            # Exact tag + title phrase - exact tag
keyword=artist:aaa tankobon -male:yaoi                  # Tag + title word - tag
keyword=language:eng o:uncensored                       # Multiple prefix tags
```

**Complex Queries:**

```
keyword=~f:schoolgirl ~f:"office lady" language:english         # (Tag f:schoolgirl OR f:office lady) AND tag language:english
keyword=artist:aaa -f:lolicon f:"big breasts$"                  # Tag artist:aaa + exact tag f:big breasts - tag f:lolicon
keyword="comic aun" -2007 -2008 o:"full color"                  # Title "comic aun" - "2007" - "2008" + tag o:full color
keyword=~c:"dark magician girl" ~c:"blue-eyes white dragon"     # OR search with multi-word character tags
keyword="Paint Lab" f:big -male:drugs parody:original$          # Complex mix of title phrase, tags, excludes
```

**Combined with Filters:**

```
keyword=o:"full%20color"&category=Doujinshi&minpage=20&minrating=4.5
keyword=female:elf%20language:chinese&minpage=100&maxdate=1704067200
keyword=tankobon&category=Manga&minrating=4.0
```

### Search Behavior

**Tag vs Title Matching:**

- **Tag Search**: Only terms containing colon (`:`) are treated as tag searches
  - Format: `namespace:tag` or `namespace:"multi word tag"`
  - Examples: `female:elf`, `artist:aaa`, `character:"dark magician girl"`
  - Searches against gallery tags
  - Supports namespace shortcuts (e.g., `f:` = `female:`, `a:` = `artist:`)
  - **Prefix Matching**: Tags **without** `$` suffix match by prefix (e.g., `female:big` matches `female:big breasts`, `female:big ass`, etc.)
  - **Exact Matching**: Tags **with** `$` suffix match exactly (e.g., `female:wolf$` matches only `female:wolf`, not `female:wolf girl`)

- **Title Search**: Terms without colon are treated as title searches
  - Format: `term` or `"phrase"` (no colon)
  - Examples: `pokemon`, `"comic aun"`, `"original work"`
  - Searches against **both English and Japanese titles** simultaneously
  - Case-insensitive, partial match (e.g., `pokemon` matches "Pokemon Special")

**Quotation Marks:**

- Quotes `"..."` are used to group multi-word phrases/terms
- For tags: `namespace:"multi word tag"` - when tag value contains spaces
- For titles: `"multi word phrase"` - for exact phrase matching in titles
- Both tag and title searches can use quotation marks

**Logic:**

- **AND Logic**: Multiple terms are combined with AND (all must match)
- **OR Logic**: Terms prefixed with `~` in the same group are combined with OR (any must match)

## Acknowledgments

This project is made possible by the following projects:

- [ccloli/e-hentai-db](https://github.com/ccloli/e-hentai-db) - Original MySQL-based E-Hentai database implementation that inspired this project
- [URenko/e-hentai-db](https://github.com/URenko/e-hentai-db) - Live crawling and regular database dumps that keep this project up-to-date

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Who wrote this crap?

<img src="https://github.com/user-attachments/assets/1c970202-1d0b-4b8b-960a-1542e5c234a5" alt="Who wrote this crap?" width="30%">
