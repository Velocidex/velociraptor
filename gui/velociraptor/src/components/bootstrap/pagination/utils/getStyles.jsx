const getStyles = (styles, className) => {
    switch (className) {
        case undefined || false:
            return {
                color: styles.color && styles.color,
                backgroundColor: styles.bgColor,
                borderColor: styles.borderColor
            }
        case 'disabled':
            return {
                color: styles.disabledColor && styles.disabledColor,
                backgroundColor: styles.disabledBgColor ? styles.disabledBgColor : styles.bgColor,
                borderColor: styles.disabledBorderColor ? styles.disabledBorderColor : styles.borderColor
            }
        case 'active':
            return {
                color: styles.activeColor && styles.activeColor,
                backgroundColor: styles.activeBgColor,
                borderColor: styles.activeBorderColor
            }
        default:
            break;
    }
}

export default getStyles;