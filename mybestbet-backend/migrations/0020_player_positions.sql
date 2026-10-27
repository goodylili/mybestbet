ALTER TABLE players ADD COLUMN IF NOT EXISTS position text;

WITH normalized (sport, raw, position) AS (VALUES
    ('football', 'G', 'GK'), ('football', 'GK', 'GK'), ('football', 'GOALKEEPER', 'GK'), ('football', 'KEEPER', 'GK'), ('football', 'KEEPERS', 'GK'), ('football', 'GOALKEEPERS', 'GK'),
    ('football', 'D', 'DF'), ('football', 'DF', 'DF'), ('football', 'DEFENDER', 'DF'), ('football', 'DEFENDERS', 'DF'),
    ('football', 'M', 'MF'), ('football', 'MF', 'MF'), ('football', 'MIDFIELDER', 'MF'), ('football', 'MIDFIELDERS', 'MF'),
    ('football', 'F', 'FW'), ('football', 'FW', 'FW'), ('football', 'FORWARD', 'FW'), ('football', 'FORWARDS', 'FW'), ('football', 'ATTACKER', 'FW'), ('football', 'ATTACKERS', 'FW'),
    ('basketball', 'G', 'G'), ('basketball', 'PG', 'G'), ('basketball', 'SG', 'G'), ('basketball', 'GUARD', 'G'), ('basketball', 'POINT GUARD', 'G'), ('basketball', 'SHOOTING GUARD', 'G'),
    ('basketball', 'F', 'F'), ('basketball', 'SF', 'F'), ('basketball', 'PF', 'F'), ('basketball', 'FORWARD', 'F'), ('basketball', 'SMALL FORWARD', 'F'), ('basketball', 'POWER FORWARD', 'F'),
    ('basketball', 'C', 'C'), ('basketball', 'CENTER', 'C'), ('basketball', 'CENTRE', 'C'),
    ('american-football', 'QB', 'QB'), ('american-football', 'RB', 'RB'), ('american-football', 'FB', 'RB'), ('american-football', 'WR', 'WR'), ('american-football', 'TE', 'TE'),
    ('american-football', 'OL', 'OL'), ('american-football', 'OT', 'OL'), ('american-football', 'T', 'OL'), ('american-football', 'OG', 'OL'), ('american-football', 'G', 'OL'), ('american-football', 'C', 'OL'),
    ('american-football', 'DEF', 'DEF'), ('american-football', 'DL', 'DEF'), ('american-football', 'DT', 'DEF'), ('american-football', 'DE', 'DEF'), ('american-football', 'NT', 'DEF'), ('american-football', 'EDGE', 'DEF'),
    ('american-football', 'LB', 'DEF'), ('american-football', 'ILB', 'DEF'), ('american-football', 'OLB', 'DEF'), ('american-football', 'MLB', 'DEF'),
    ('american-football', 'DB', 'DEF'), ('american-football', 'CB', 'DEF'), ('american-football', 'S', 'DEF'), ('american-football', 'SS', 'DEF'), ('american-football', 'FS', 'DEF'),
    ('american-football', 'K', 'K'), ('american-football', 'PK', 'K'), ('american-football', 'P', 'P'), ('american-football', 'LS', 'LS')
),
current AS (
    SELECT p.id, n.position
    FROM players p
    LEFT JOIN teams t ON t.id = p."teamId"
    LEFT JOIN leagues l ON l.code = t."leagueCode"
    LEFT JOIN normalized n ON n.sport = l.sport AND n.raw = split_part(upper(btrim(p.position)), '-', 1)
    WHERE p.position IS NOT NULL
)
UPDATE players p SET position = current.position
FROM current
WHERE p.id = current.id AND p.position IS DISTINCT FROM current.position;

ALTER TABLE players DROP CONSTRAINT IF EXISTS players_position_check;
ALTER TABLE players ADD CONSTRAINT players_position_check
    CHECK (position IN ('GK', 'DF', 'MF', 'FW', 'G', 'F', 'C', 'QB', 'RB', 'WR', 'TE', 'OL', 'DEF', 'K', 'P', 'LS'));
