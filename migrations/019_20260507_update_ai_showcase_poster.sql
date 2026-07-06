UPDATE ai_showcase_projects 
SET poster_url = CONCAT(ai_showcase_link, '.webp')
WHERE id > 0;
