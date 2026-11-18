CREATE TABLE dbo.sample_items (id int PRIMARY KEY, label nvarchar(40) NOT NULL);
INSERT INTO dbo.sample_items (id, label) VALUES (1, N'one'), (2, N'two');
