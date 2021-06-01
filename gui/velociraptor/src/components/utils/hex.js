import React from 'react';
import PropTypes from 'prop-types';
import _ from 'lodash';
import Button from 'react-bootstrap/Button';
import { FontAwesomeIcon } from '@fortawesome/react-fontawesome';


// A hex viewer suitable for small amountfs of text - No paging.
export default class HexView extends React.Component {
    static propTypes = {
        data: PropTypes.string,
        height: PropTypes.number,
    };

    state = {
        rows: 25,
        columns: 0x10,
        hexDataRows: [],
        parsed_data: "",
        expanded: false,
    }

    componentDidMount = () => {
        this.parseFileContentToHexRepresentation_(this.props.data);
        this.setState({parsed_data: this.props.data});
    }

    componentDidUpdate = (prevProps, prevState, rootNode) => {
        return this.props.data !== this.state.parsed_data;
    }

    parseFileContentToHexRepresentation_ = (fileContent) => {
        if (!fileContent) {
            fileContent = "";
        }
        let hexDataRows = [];
        for(var i = 0; i < this.state.rows; i++){
            let offset = 0;
            var rowOffset = offset + (i * this.state.columns);
            var data = fileContent.substr(i * this.state.columns, this.state.columns);
            var data_row = [];
            for (var j = 0; j < data.length; j++) {
                var char = data.charCodeAt(j).toString(16);
                data_row.push(('0' + char).substr(-2)); // add leading zero if necessary
            };

            if (data_row.length === 0) {
                break;
            };

            hexDataRows.push({
                offset: rowOffset,
                data_row: data_row,
                data: data,
                safe_data: data.replace(/[^\x20-\x7f]/g, '.'),
            });

        }

        this.setState({hexDataRows: hexDataRows, loading: false});
    };


    render() {
        let height = this.props.height || 5;
        let more = this.state.hexDataRows.length > height;
        let hexArea =
            <table className="hex-area">
              <tbody>
                { _.map(this.state.hexDataRows, (row, idx)=>{
                    if (idx >= height && !this.state.expanded) {
                        return <></>;
                    }
                    return <tr key={idx}>
                             <td>
                               { _.map(row.data_row, (x, idx)=>{
                                   return <span key={idx}>{ x }&nbsp;</span>;
                               })}
                             </td>
                           </tr>; })
                }
              </tbody>
            </table>;

        let contextArea =
            <table className="content-area">
              <tbody>
                { _.map(this.state.hexDataRows, (row, idx)=>{
                    if (idx >= height && !this.state.expanded) {
                        return <></>;
                    }
                    return <tr key={idx}><td className="data">{ row.safe_data }</td></tr>;
                })}
              </tbody>
            </table>;

        return (
            <div>
              <div className="file-hex-view">
                <div className="panel hexdump">
                  <div className="monospace">
                    <table>
                      <thead>
                        <tr>
                          <th>Offset</th>
                          <th>00 01 02 03 04 05 06 07 08 09 0a 0b 0c 0d 0e 0f</th>
                          <th></th>
                        </tr>
                      </thead>
                      <tbody>
                        <tr>
                          <td>
                            <table className="offset-area">
                              <tbody>
                                { _.map(this.state.hexDataRows, (row, idx)=>{
                                    if (idx >= height && !this.state.expanded) {
                                        return <></>;
                                    }
                                    return <tr key={idx}>
                                             <td className="offset">
                                               { row.offset }
                                             </td>
                                           </tr>; })}
                              </tbody>
                            </table>
                          </td>
                          <td>
                            { hexArea }
                          </td>
                          <td>
                            { contextArea }
                          </td>
                        </tr>
                        { more && (this.state.expanded ?
                                   <tr>
                                     <td colspan="16">
                                       <Button variant="default-outline" title="Collapse"
                                               onClick={()=>this.setState({expanded: false})} >
                                         <i><FontAwesomeIcon icon="arrow-up"/></i>
                                       </Button>
                                     </td>
                                   </tr>
                                   : <tr>
                                       <td colspan="16">
                                         <Button variant="default-outline"  title="Expand"
                                                 onClick={()=>this.setState({expanded: true})} >
                                           <i><FontAwesomeIcon icon="arrow-down"/></i>
                                         </Button>
                                       </td>
                                     </tr>) }
                      </tbody>
                    </table>
                  </div>
                </div>
              </div>
            </div>
        );
    }
};
