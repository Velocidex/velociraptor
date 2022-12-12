import React from 'react';
import getStyles from './utils/getStyles';

const PageItem = ({
    text,
    page,
    className,
    onClick,
    href,
    activeBgColor,
    activeBorderColor,
    disabledBgColor,
    disabledBorderColor,
    bgColor,
    borderColor,
    activeColor,
    disabledColor,
    color,
    circle,
    shadow,
    size
}) => (
        <li className={`page-item ${className !== undefined && className !== false ? className : ''}`} >
            <a
                style={{
                    ...getStyles({
                        activeBgColor,
                        activeBorderColor,
                        disabledBgColor,
                        disabledBorderColor,
                        bgColor,
                        borderColor,
                        activeColor,
                        color,
                        disabledColor
                    }, className),
                    ...circleStyle(circle, size),
                    ...shadowStyle(shadow, circle)

                }}
                className={'page-link'}
                onClick={(e) => onClick && onClick(page, e)}
                {...onClick ? { href: '#' } : { href: href }} >
                {text}
            </a>
        </li >
    );

const circleStyle = (isCircle, size) => {
    if (!isCircle) return {}
    if (size === 'lg' || size === 'sm') {
        if (size === 'lg') {
            return {
                borderRadius: '30px',
                marginLeft: '6px',
                marginRight: '6px',
                width: '57px',
                height: '57px',
                padding: '.75rem 17px'
            }
        }
        if (size === 'sm') {
            return {
                borderRadius: '30px',
                marginLeft: '4px',
                marginRight: '4px',
                width: '36px',
                height: '36px',
                padding: '7px'
            }
        }
    } else {
        return {
            borderRadius: '30px',
            marginLeft: '6px',
            marginRight: '6px',
            width: '45px',
            height: '45px',
            padding: '11px'
        }
    }
}

const shadowStyle = (showShadow, isCircle) => {
    if (!showShadow) return {}
    if (!isCircle) return {}
    return {
        WebkitBoxShadow: '0 8px 17px 0 rgba(0,0,0,0.2),0 6px 20px 0 rgba(0,0,0,0.19)',
        boxShadow: '0px 8px 17px 0px rgba(0,0,0,0.2),0px 6px 20px 0px rgba(0,0,0,0.19)'
    }
}

export default PageItem;
