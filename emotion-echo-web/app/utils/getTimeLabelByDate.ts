import type { conversationListLabelType } from "~/types/conversation/conversationListType";

export const getTimeLabelByDate = (
  date: Date
): Exclude<conversationListLabelType, "置顶"> => {
  const now = Date.now();
  const targetTime = date.getTime();
  const timeDiff = now - targetTime; // 距今的时间差（毫秒）

  const ONE_DAY = 24 * 60 * 60 * 1000;
  const SEVEN_DAYS = ONE_DAY * 7;
  const THIRTY_DAYS = ONE_DAY * 30;

  if (timeDiff <= ONE_DAY) {
    return "今天";
  } else if (timeDiff <= SEVEN_DAYS) {
    return "一周内";
  } else if (timeDiff <= THIRTY_DAYS) {
    return "三十天内";
  } else {
    return "更早";
  }
};
