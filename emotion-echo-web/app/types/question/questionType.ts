export interface questionData {
  id: number;
  name: string;
  finishQuesTime: string;
  questionStatus: questionStatus;
}
export interface surveyData{
    id:number;
    title:string;
    description?:string;
    questions:Array<radioQuestionData>|[];
}
export interface radioQuestionData {
    id: number;
    name: string;
    options: Array<optionType>;
}
type optionType = {
    id:number;
    text:string;
};
type questionStatus = "已完成" | "未开始" | "进行中";
