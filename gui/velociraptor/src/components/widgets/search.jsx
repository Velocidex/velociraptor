import "./search.css";

import _ from 'lodash';
import {CancelToken} from 'axios';

import PropTypes from 'prop-types';
import React from 'react';
import Pagination from 'react-bootstrap/Pagination';
import Form from 'react-bootstrap/Form';
import T from '../i8n/i8n.jsx';

import api from '../core/api-service.jsx';


export default class SearchHex extends React.Component {
    static propTypes = {
        base_offset: PropTypes.number,
        vfs_components: PropTypes.array,
        current_page: PropTypes.number,
        page_size:  PropTypes.number,
        onPageChange: PropTypes.func,
        set_highlights: PropTypes.func,
        byte_array: PropTypes.any,
        version: PropTypes.any,
    }

    state = {
        search_type: "regex",
        search_term: "",
        search_term_error: "",
        loading: false,
    }

    componentDidMount = () => {
        this.source = CancelToken.source();
    }

    componentWillUnmount() {
        this.source.cancel();
    }

    componentDidUpdate = (prevProps, prevState, rootNode) => {
        if (!_.isEqual(prevProps.version, this.props.version) ||
            !_.isEqual(prevState.search_term, this.state.search_term)) {
            this.updateHighlights(this.state.search_term,
                                  this.state.search_type);
        };
    }

    processHit = response=>{
        if (response.cancel) return;

        if(_.isEmpty(response.data.vfs_components) || !response.data) {
            api.error("Error: No match found");
            return;
        }

        let hit_page = parseInt(response.data.hit /
                                this.props.page_size);
        this.props.onPageChange(hit_page);
        this.setState({loading: false});
    }

    searchNext = ()=>{
        // Cancel any in flight calls.
        this.source.cancel();
        this.source = CancelToken.source();

        if (!this.props.vfs_components) {
            return;
        }

        this.setState({loading: true});
        api.post("v1/SearchFile", {
            term: this.state.search_term,
            type: this.state.search_type,
            vfs_components: this.props.vfs_components,

            // Start searching from the next page
            offset: this.props.base_offset + this.props.page_size,
            forward: true,
        }, this.source.token).then(this.processHit);
    }

    searchPrev = ()=>{
        // Cancel any in flight calls.
        this.source.cancel();
        this.source = CancelToken.source();

        if (!this.props.vfs_components) {
            return;
        }

        this.setState({loading: true});
        api.post("v1/SearchFile", {
            term: this.state.search_term,
            type: this.state.search_type,
            vfs_components: this.props.vfs_components,

            // Start searching from the next page
            offset: this.props.base_offset,
            forward: false,
        }, this.source.token).then(this.processHit);
    }

    updateHighlights = (search_term, search_type)=>{
        if (!search_term) {
            return;
        }

        if (search_type === "string") {
            this.updateHighlightsForString(search_term);

        } else if (search_type === "regex") {
            this.updateHighlightsForRegex(search_term);

        } else if(search_type ==="hex") {
            search_term = search_term.replaceAll(" ", "");
            let decoded = "";
            for (let i = 0; i < search_term.length; i += 2) {
                let char = search_term.substr(i, 2);
                if(char.match(/[0-9a-f]{2}/i)) {
                    decoded += String.fromCharCode(parseInt(char, 16));
                } else {
                    this.setState({search_term_error: "Invalid hex string"});
                    this.props.set_highlights("search", []);
                    return;
                }
            }

            if (decoded) {
                this.setState({search_term_error: ""});
                this.updateHighlightsForString(decoded);
            }
        }
    }

    updateHighlightsForString = search_term=>{
        let hits = [];
        let utf8_buffer = new TextDecoder('latin1').decode(
            this.props.byte_array);
        let start = -1;
        for(;;) {
            let idx = utf8_buffer.indexOf(search_term, start+1);
            if (idx<0) {
                break;
            }
            hits.push({start: idx + this.props.base_offset,
                       end: idx+ this.props.base_offset +
                       search_term.length});
            start = idx+1;
        }
        this.props.set_highlights("search", hits);
        return;
    }

    updateHighlightsForRegex = search_term=>{
        let hits = [];
        let utf8_buffer = new TextDecoder('latin1').decode(this.props.byte_array);
        let re = undefined;
        try {
            re = new RegExp(search_term, "smgi");
            this.setState({search_term_error: ""});

        } catch(e) {
            this.setState({search_term_error: e.message});
            this.props.set_highlights("search", []);
            return;
        };

        for(;;) {
            let m = re.exec(utf8_buffer);
            if (!m) {
                break;
            }
            hits.push({start: m.index + this.props.base_offset,
                       end: m.index + m[0].length + this.props.base_offset});
        }
        this.props.set_highlights("search", hits);
        return;
    }

    render() {
        return <Pagination className="mb-3">
                 { this.state.search_term_error &&
                   <div className="search-term-error">
                     {T(this.state.search_term_error)}
                   </div>}
                 <Pagination.Prev
                   disabled={!this.state.search_term}
                   onClick={()=>this.searchPrev()}/>
                 <Form.Control as="select"
                               className="hexview-search-type-selector pagination page-link"
                               placeholder={T("Type")}
                               onChange={e=>{
                                   this.setState({search_type: e.target.value});
                                   this.updateHighlights(
                                       this.state.search_term, e.target.value);
                               }}
                               value={this.state.search_type}>
                   <option value="regex">Regex</option>
                   <option value="string">String</option>
                   <option value="hex">Hex</option>
                 </Form.Control>
                 <Form.Control
                   value={this.state.search_term}
                   className="pagination page-link hexview-search-input"
                   onChange={(e)=>{
                       this.setState({search_term: e.target.value});
                   }}
                   placeholder={T("Search for string or hex")}
                 />
                 <Pagination.Next
                   disabled={!this.state.search_term}
                   onClick={()=>this.searchNext()}/>
               </Pagination>;
    }
}
