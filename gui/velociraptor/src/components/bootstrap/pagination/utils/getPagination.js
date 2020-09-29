'use strict'

const getPagination = (props) => {

    let arr = [];
    let startAt = props.currentPage - Math.floor(props.showMax / 2);
    let isPositive = () => Math.sign(startAt);
    if (isPositive() === -1 || isPositive() === 0 || isPositive() === -0) startAt = 1;
    let max = startAt + props.showMax;
    if (max > props.totalPages) {
        max = props.totalPages;
    }

    for (let i = startAt; i < max; i++) {
        arr.push({
            page:  i,
            text:  i,
            isCurrent: isCurrent(i, props.currentPage),
            class: isCurrent(i, props.currentPage) ? props.activeClass : props.defaultClass,
            href: i === 1 ? props.href && props.pageOneHref : props.href && props.href.replace('*', startAt + i)
        });
    }

    if (props.threeDots) {
        if (arr.length) {
            arr = addThreeDots(arr, props);
        }
    }

    if (props.prevNext) {
        if (arr.length) {
            arr = addNext(arr, props);
            arr = addPrev(arr, props);
        }
    }

    return arr;
}

const isCurrent = (page, currentPage) => currentPage === page;

const addThreeDots = (arr, props) => {
    let threeDotsObj = { page: false, text: '...', isCurrent: false, class: props.disabledClass };
    arr[0].page !== 1 && arr.unshift(threeDotsObj) && arr.unshift({ page: 1, text: 1, isCurrent: false, class: props.defaultClass, href: props.pageOneHref ? props.pageOneHref : props.href && props.href.replace('*', 1) });
    arr[arr.length - 1].page !== props.totalPages && arr.push(threeDotsObj) && arr.push({ page: props.totalPages, text: props.totalPages, isCurrent: false, class: props.defaultClass, href: props.href && props.href.replace('*', props.totalPages) });
    return arr;
}

const addNext = (arr, props) => {
    let nextObj = {
        page: props.currentPage + 1,
        text: props.nextText,
        isCurrent: false,
        class: props.currentPage + 1 > props.totalPages && props.disabledClass,
        href: props.href && props.href.replace('*', props.currentPage + 1)
    };
    arr.push(nextObj);
    return arr;
}

const addPrev = (arr, props) => {
    let prevObj = {
        page: props.currentPage - 1,
        text: props.prevText,
        isCurrent: false,
        class: props.currentPage - 1 < 1 && props.disabledClass,
        href: props.href && props.href.replace('*', props.currentPage - 1)
    };
    arr.unshift(prevObj);
    return arr;
}

export {
    getPagination
}
