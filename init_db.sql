-- Create database if not exists
CREATE DATABASE IF NOT EXISTS cronconverter;

-- Connect to the database
\c cronconverter;

-- Create table for cron expressions
CREATE TABLE IF NOT EXISTS cron_expressions (
    id SERIAL PRIMARY KEY,
    name VARCHAR(255) NOT NULL,
    expression VARCHAR(255) NOT NULL,
    description TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Create indexes
CREATE INDEX IF NOT EXISTS idx_cron_expressions_name ON cron_expressions (name);
CREATE INDEX IF NOT EXISTS idx_cron_expressions_created_at ON cron_expressions (created_at);

-- Insert some sample presets
INSERT INTO cron_expressions (name, expression, description) 
VALUES 
    ('Daily at midnight', '0 0 * * *', 'Runs every day at midnight'),
    ('Hourly', '0 * * * *', 'Runs at the beginning of every hour'),
    ('Every 15 minutes', '*/15 * * * *', 'Runs every 15 minutes'),
    ('Weekdays at 9am', '0 9 * * 1-5', 'Runs at 9am on weekdays'),
    ('Monthly backup', '0 0 1 * *', 'Runs at midnight on the first day of each month');