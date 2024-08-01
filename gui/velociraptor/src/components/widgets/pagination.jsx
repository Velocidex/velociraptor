import PropTypes from 'prop-types';
import React from 'react';
import Pagination from 'react-bootstrap/Pagination';
import Form from 'react-bootstrap/Form';
import T from '../i8n/i8n.jsx';
import classNames from "classnames";


export class TextPaginationControl extends React.Component {
    static propTypes = {
        base_offset: PropTypes.number,
        setBaseOffset: PropTypes.func,

        page_size:  PropTypes.number,
        total_pages: PropTypes.number,
        current_page: PropTypes.number,
        total_size: PropTypes.number,
        onPageChange: PropTypes.func,
    }

    state = {
        next_offset: 0,
    }

    render() {
        let last_page = this.props.total_pages-1;
        if (last_page <= 0) {
            last_page = 0;
        }

        return (
            <Pagination className="hex-goto">
              <Pagination.First
                disabled={this.props.current_page===0}
                onClick={()=>this.gotoPage(0)}/>
              <Pagination.Prev
                disabled={this.props.current_page===0}
                onClick={()=>this.gotoPage(this.props.current_page-1)}/>

              <Form.Control
                as="input"
                className={classNames({
                    "page-link": true,
                    "goto-invalid": this.state.goto_error,
                })}
                id="goto_page"
                placeholder={T("Goto Offset")}
                spellCheck="false"
                value={this.state.goto_offset}
                onChange={e=> {
                    let goto_offset = e.target.value;
                    this.setState({goto_offset: goto_offset});

                    if (goto_offset === "") {
                        return;
                    }

                    let base_offset = parseInt(goto_offset);
                    if (isNaN(base_offset)) {
                        this.setState({goto_error: true});
                        return;
                    }
                    this.setState({goto_error: false});

                    if (base_offset > this.props.total_size) {
                        goto_offset = this.props.total_size;
                        base_offset = this.props.total_size;
                        this.setState({
                            goto_offset: goto_offset,
                        });
                    }
                    this.setHighlight(base_offset);

                    let page = parseInt(base_offset/this.props.page_size);
                    this.props.onPageChange(page);
                }}/>
              <Pagination.Next
                disabled={last_page == 0 || this.props.current_page===last_page}
                onClick={()=>this.gotoPage(this.props.current_page+1)}/>
              <Pagination.Last
                disabled={last_page == 0 || this.props.current_page===last_page}
                onClick={()=>this.gotoPage(last_page)}/>
            </Pagination>
        );
    }
}


export default class HexPaginationControl extends React.Component {
    static propTypes = {
        total_pages: PropTypes.number,
        page_size:  PropTypes.number,
        total_size: PropTypes.number,
        current_page: PropTypes.number,
        onPageChange: PropTypes.func,
        showGoToPage: PropTypes.bool,
        hex_offset: PropTypes.bool,
        set_highlights: PropTypes.func,
    }

    state = {
        goto_offset: "",
        goto_error: false,
    }

    gotoPage = page=>{
        let new_offset = page*this.props.page_size;
        if (this.props.hex_offset) {
            new_offset = "0x" + new_offset.toString(16);
        }
        this.setState({goto_offset: new_offset});
        this.clearHighlight();
        this.props.onPageChange(page);
    }

    clearHighlight = ()=>{
        if(this.props.set_highlights) {
            this.props.set_highlights("offset", []);
        }
    }

    setHighlight = offset=>{
        if(this.props.set_highlights) {
            this.props.set_highlights("offset", [{
                start: offset,
                end: offset+4}]);
        }
    }

    render() {
        let last_page = this.props.total_pages-1;
        if (last_page <= 0) {
            last_page = 0;
        }
        return (
            <Pagination className="hex-goto">
              <Pagination.First
                disabled={this.props.current_page===0}
                onClick={()=>this.gotoPage(0)}/>
              <Pagination.Prev
                disabled={this.props.current_page===0}
                onClick={()=>this.gotoPage(this.props.current_page-1)}/>

              <Form.Control
                as="input"
                className={classNames({
                    "page-link": true,
                    "goto-invalid": this.state.goto_error,
                })}
                placeholder={T("Goto Offset")}
                spellCheck="false"
                value={this.state.goto_offset}
                onChange={e=> {
                    let goto_offset = e.target.value;
                    this.setState({goto_offset: goto_offset});

                    if (goto_offset === "") {
                        return;
                    }

                    let base_offset = parseInt(goto_offset);
                    if (isNaN(base_offset)) {
                        this.setState({goto_error: true});
                        return;
                    }
                    this.setState({goto_error: false});

                    if (base_offset > this.props.total_size) {
                        goto_offset = this.props.total_size;
                        base_offset = this.props.total_size;
                        this.setState({
                            goto_offset: goto_offset,
                        });
                    }
                    this.setHighlight(base_offset);

                    let page = parseInt(base_offset/this.props.page_size);
                    this.props.onPageChange(page);
                }}/>
              <Pagination.Next
                disabled={last_page == 0 || this.props.current_page===last_page}
                onClick={()=>this.gotoPage(this.props.current_page+1)}/>
              <Pagination.Last
                disabled={last_page == 0 || this.props.current_page===last_page}
                onClick={()=>this.gotoPage(last_page)}/>
            </Pagination>
        );
    }
}
