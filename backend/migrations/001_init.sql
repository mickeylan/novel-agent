-- Novel Agent Database Schema
-- Run this script to initialize the database

-- Create database (run as superuser)
-- CREATE DATABASE novel_agent;
-- CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
-- CREATE EXTENSION IF NOT EXISTS "vector";

-- Create schema
CREATE SCHEMA IF NOT EXISTS novel_agent;

-- Set search path
SET search_path TO novel_agent, public;

-- ========== Books Table ==========
CREATE TABLE IF NOT EXISTS books (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title VARCHAR(255) NOT NULL,
    author VARCHAR(255),
    genre TEXT[],
    language VARCHAR(20) DEFAULT 'zh',
    word_count INTEGER,
    file_path VARCHAR(500),
    file_type VARCHAR(50),
    processed_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

-- ========== Book Chunks Table ==========
CREATE TABLE IF NOT EXISTS book_chunks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    book_id UUID NOT NULL REFERENCES books(id) ON DELETE CASCADE,
    chapter INTEGER,
    section INTEGER,
    content TEXT NOT NULL,
    embedding VECTOR(768),
    metadata JSONB DEFAULT '{}',
    word_count INTEGER,
    category VARCHAR(100) DEFAULT '其他',
    created_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_book_chunks_book_id ON book_chunks(book_id);
CREATE INDEX IF NOT EXISTS idx_book_chunks_embedding ON book_chunks USING ivfflat (embedding vector_cosine_ops);

-- ========== Story Projects Table ==========
CREATE TABLE IF NOT EXISTS story_projects (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title VARCHAR(255) NOT NULL,
    genre TEXT[],
    status VARCHAR(50) DEFAULT 'draft',
    description TEXT,
    outline TEXT,
    protagonist VARCHAR(255),
    core_setting TEXT,
    total_chapters INTEGER DEFAULT 1000,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_story_projects_status ON story_projects(status);
CREATE INDEX IF NOT EXISTS idx_story_projects_created_at ON story_projects(created_at DESC);

-- ========== Characters Table ==========
CREATE TABLE IF NOT EXISTS characters (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES story_projects(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    traits TEXT[],
    relationships JSONB DEFAULT '{}',
    arc_summary TEXT,
    current_state VARCHAR(255),
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_characters_project_id ON characters(project_id);
CREATE INDEX IF NOT EXISTS idx_characters_name ON characters(project_id, name);

-- ========== Chapters Table ==========
CREATE TABLE IF NOT EXISTS chapters (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES story_projects(id) ON DELETE CASCADE,
    chapter_number INTEGER NOT NULL,
    title VARCHAR(255),
    outline TEXT,
    content TEXT,
    status VARCHAR(50) DEFAULT 'draft',
    summary TEXT,
    key_events TEXT[],
    new_characters TEXT,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    UNIQUE(project_id, chapter_number)
);

CREATE INDEX IF NOT EXISTS idx_chapters_project_id ON chapters(project_id);
CREATE INDEX IF NOT EXISTS idx_chapters_number ON chapters(project_id, chapter_number);
CREATE INDEX IF NOT EXISTS idx_chapters_status ON chapters(project_id, status);

-- ========== Foreshadowings Table ==========
CREATE TABLE IF NOT EXISTS foreshadowings (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES story_projects(id) ON DELETE CASCADE,
    title VARCHAR(255) NOT NULL,
    description TEXT NOT NULL,
    chapter_created INTEGER NOT NULL,
    foreshadowing_type VARCHAR(50) DEFAULT 'plot',
    importance INTEGER DEFAULT 3,
    content_snippet TEXT,
    estimated_payoff_chapter INTEGER,
    status VARCHAR(50) DEFAULT 'active',
    payoff_chapter INTEGER,
    payoff_description TEXT,
    tags TEXT[],
    notes TEXT,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_foreshadowings_project_id ON foreshadowings(project_id);
CREATE INDEX IF NOT EXISTS idx_foreshadowings_status ON foreshadowings(project_id, status);

-- ========== Particle Ledger Table ==========
CREATE TABLE IF NOT EXISTS particle_ledger (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES story_projects(id) ON DELETE CASCADE,
    character_name VARCHAR(255) NOT NULL,
    resource_type VARCHAR(50) NOT NULL,
    resource_name VARCHAR(255) NOT NULL,
    current_value VARCHAR(255),
    previous_value VARCHAR(255),
    chapter_number INTEGER,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_particle_ledger_project ON particle_ledger(project_id);
CREATE INDEX IF NOT EXISTS idx_particle_ledger_character ON particle_ledger(project_id, character_name);

-- ========== Pending Hooks Table ==========
CREATE TABLE IF NOT EXISTS pending_hooks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES story_projects(id) ON DELETE CASCADE,
    chapter_id UUID REFERENCES chapters(id) ON DELETE SET NULL,
    title VARCHAR(255) NOT NULL,
    description TEXT,
    hook_type VARCHAR(50) DEFAULT 'foreshadowing',
    priority VARCHAR(50) DEFAULT 'medium',
    introduced_chapter INTEGER NOT NULL,
    expected_resolution_chapter INTEGER,
    resolved_chapter INTEGER,
    resolution_description TEXT,
    status VARCHAR(50) DEFAULT 'active',
    related_characters TEXT[],
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_pending_hooks_project ON pending_hooks(project_id);
CREATE INDEX IF NOT EXISTS idx_pending_hooks_status ON pending_hooks(project_id, status);

-- ========== Subplot Board Table ==========
CREATE TABLE IF NOT EXISTS subplot_board (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES story_projects(id) ON DELETE CASCADE,
    subplot_name VARCHAR(255) NOT NULL,
    subplot_type VARCHAR(50),
    description TEXT,
    status VARCHAR(50) DEFAULT 'active',
    start_chapter INTEGER NOT NULL,
    current_chapter INTEGER,
    expected_end_chapter INTEGER,
    progress_percentage INTEGER DEFAULT 0,
    key_events TEXT[],
    related_characters TEXT[],
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_subplot_board_project ON subplot_board(project_id);
CREATE INDEX IF NOT EXISTS idx_subplot_board_status ON subplot_board(project_id, status);

-- ========== Emotional Arcs Table ==========
CREATE TABLE IF NOT EXISTS emotional_arcs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES story_projects(id) ON DELETE CASCADE,
    character_name VARCHAR(255) NOT NULL,
    arc_type VARCHAR(50),
    starting_state VARCHAR(255),
    current_state VARCHAR(255),
    target_state VARCHAR(255),
    key_turning_points TEXT[],
    chapter_progress INTEGER DEFAULT 0,
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_emotional_arcs_project ON emotional_arcs(project_id);
CREATE INDEX IF NOT EXISTS idx_emotional_arcs_character ON emotional_arcs(project_id, character_name);

-- ========== Audit Results Table ==========
CREATE TABLE IF NOT EXISTS audit_results (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    chapter_id UUID NOT NULL REFERENCES chapters(id) ON DELETE CASCADE,
    project_id UUID NOT NULL REFERENCES story_projects(id) ON DELETE CASCADE,
    auditor_type VARCHAR(50),
    audit_dimensions JSONB DEFAULT '{}',
    issues JSONB DEFAULT '[]',
    overall_score DECIMAL(5,2),
    status VARCHAR(50) DEFAULT 'pending',
    revision_count INTEGER DEFAULT 0,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_audit_results_chapter ON audit_results(chapter_id);
CREATE INDEX IF NOT EXISTS idx_audit_results_project ON audit_results(project_id);

-- ========== Writing Styles Table ==========
CREATE TABLE IF NOT EXISTS writing_styles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    project_id UUID NOT NULL REFERENCES story_projects(id) ON DELETE CASCADE,
    name VARCHAR(255) NOT NULL,
    style_preset VARCHAR(50) DEFAULT 'balanced',
    prose_density VARCHAR(50) DEFAULT 'medium',
    pacing VARCHAR(50) DEFAULT 'moderate',
    description_focus TEXT[],
    dialogue_style VARCHAR(50) DEFAULT 'natural',
    narration_voice VARCHAR(50) DEFAULT 'third_person',
    custom_instructions TEXT,
    user_preferences JSONB DEFAULT '{}',
    lessons_learned JSONB DEFAULT '[]',
    style_metrics_history JSONB DEFAULT '[]',
    reader_feedback JSONB DEFAULT '[]',
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_writing_styles_project ON writing_styles(project_id);

-- Grant permissions
-- GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA novel_agent TO novel_agent;
-- GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA novel_agent TO novel_agent;
