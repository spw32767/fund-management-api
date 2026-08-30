package models


// InstructorFullProfile คือก้อนข้อมูลรวมที่จะส่งไปให้ Frontend
type InstructorFullProfile struct {
    Header     InstructorProfileHeader  `json:"header"`
    Educations []InstructorEducationTab `json:"educations"`
    Expertises []InstructorExpertiseTab `json:"expertises"`
    Courses    []InstructorCourse       `json:"courses"` 
	RankingSources []RankingSource `json:"ranking_sources"` 
	RankingTierWeights []RankingTierWeight `json:"ranking_tier_weights"` 
	InstructorEditLogs []InstructorEditLog `json:"instructor_edit_logs"` 
	InstructorTextbooks []InstructorTextbook `json:"instructor_textbooks"` 
	InstructorResearchProjects []InstructorResearchProject `json:"instructor_research_projects"` 
	InstructorDegrees []InstructorDegree `json:"instructor_degrees"`
	InstructorCourseResponsibility []InstructorCourseResponsibility `json:"instructor_course_responsibility"` 
	InstructorIntellectualProperties []InstructorIntellectualProperty `json:"instructor_intellectual_properties"`
}
