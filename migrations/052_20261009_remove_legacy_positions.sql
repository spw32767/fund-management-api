-- Keep the academic title in users.position while retiring the legacy position lookup.
-- Rebuild the only view that joins positions before dropping the table.
UPDATE users AS u
JOIN positions AS p ON p.position_id = u.position_id
SET u.position = p.position_name
WHERE (u.position IS NULL OR TRIM(u.position) = '')
  AND p.position_name IS NOT NULL;

CREATE OR REPLACE VIEW view_fund_applications_summary AS
SELECT fa.application_id,
       fa.application_number,
       fa.project_title,
       CONCAT(u.user_fname, ' ', u.user_lname) AS applicant_name,
       u.email,
       u.position AS position_name,
       fc.category_name,
       fs.subcategory_name,
       y.year AS year,
       ast.status_name,
       fa.requested_amount,
       fa.approved_amount,
       fa.submitted_at,
       fa.approved_at
FROM v_fund_applications AS fa
LEFT JOIN users AS u ON fa.user_id = u.user_id
LEFT JOIN fund_subcategories AS fs ON fa.subcategory_id = fs.subcategory_id
LEFT JOIN fund_categories AS fc ON fs.category_id = fc.category_id
LEFT JOIN years AS y ON fa.year_id = y.year_id
LEFT JOIN application_status AS ast ON fa.application_status_id = ast.application_status_id
WHERE fa.delete_at IS NULL;

ALTER TABLE users DROP FOREIGN KEY users_ibfk_2;
ALTER TABLE users DROP INDEX position_id;
ALTER TABLE users DROP COLUMN position_id;
DROP TABLE positions;
