package models


// InstructorFullProfile คือก้อนข้อมูลรวมที่จะส่งไปให้ Frontend
type InstructorFullProfile struct {
    Header     InstructorProfileHeader  `json:"header"`
    Educations []InstructorEducationTab `json:"educations"`
    Expertises []InstructorExpertiseTab `json:"expertises"`
    Courses    []InstructorCourse       `json:"courses"` // ถ้าอยากให้แสดงหลักสูตรที่สอนด้วย
	RankingSources []RankingSource `json:"ranking_sources"` // ถ้าอยากให้แสดงแหล่งจัดอันดับด้วย
	RankingTierWeights []RankingTierWeight `json:"ranking_tier_weights"` // ถ้าอยากให้แสดงน้ำหนักคะแนนจัดอันดับด้วย
	InstructorEditLogs []InstructorEditLog `json:"instructor_edit_logs"` // ถ้าอยากให้แสดงประวัติการแก้ไขด้วย
	InstructorTextbooks []InstructorTextbook `json:"instructor_textbooks"` // ถ้าอยากให้แสดงหนังสือที่แต่งด้วย
	InstructorResearchProjects []InstructorResearchProject `json:"instructor_research_projects"` // ถ้าอยากให้แสดงโครงการวิจัยด้วย
	InstructorDegrees []InstructorDegree `json:"instructor_degrees"` // ถ้าอยากให้แสดงวุฒิการศึกษาด้วย
	InstructorCourseResponsibility []InstructorCourseResponsibility `json:"instructor_course_responsibility"` // ถ้าอยากให้แสดงความรับผิด
	InstructorIntellectualProperties []InstructorIntellectualProperty `json:"instructor_intellectual_properties"`
}
