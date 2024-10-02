import _ from 'lodash';
import React, { createRef, Component } from 'react';
import PropTypes from 'prop-types';


export default class ColumnResizer extends Component {
    ref = createRef();

    static propTypes = {
        // Called to set the width of the previous column.
        setWidth: PropTypes.func,
        width: PropTypes.number,
        className: PropTypes.string,
    }

    componentDidMount() {
        document.addEventListener('mousemove', this.onMouseMove);
        document.addEventListener('mouseup', this.endDrag);
        this.dragging = false;

        // Track the position of the mouse pointer always. It will
        // only be used when we start to drag.
        this.mouseX = 0;
    }

    componentWillUnmount() {
        document.removeEventListener('mousemove', this.onMouseMove);
        document.removeEventListener('mouseup', this.endDrag);
    }

    // Start dragging locks in the current screen position and the
    // width of the previous td.
    startDrag = e=>{
        this.startWidth = this.props.width;
        if (_.isUndefined(this.startWidth)) {
            let prev = this.ref.current.previousElementSibling;
            if(prev) {
                this.startWidth = prev.clientWidth;
            }
        }

        this.dragging = true;
        this.startingX = this.mouseX;

        // Disable selection
        e.preventDefault();
        return false;
    }

    endDrag = ()=>{
        this.dragging = false;
    }

    onMouseMove = e=>{
        this.mouseX = e.screenX;

        if (!this.dragging) {
            return;
        }

        // Resize the previous element according to the startingX and
        // current position.
        let diff = this.startingX - this.mouseX;
        let newPrevWidth = this.startWidth - diff;
        if (newPrevWidth < 0) {
            newPrevWidth = 0;
        }

        this.props.setWidth(newPrevWidth);
    }

    render() {
        let clsname = "column-resizer ";
        if (this.props.className) {
            clsname += this.props.className;
        }
        return (
            <td
              ref={this.ref}
              className={clsname}
              onMouseDown={this.startDrag}
            />
        );
    }
}
